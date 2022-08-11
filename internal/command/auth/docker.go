package auth

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/iostreams"
)

func newDocker() *cobra.Command {
	const (
		long = `Adds registry.fly.io to the docker daemon's authenticated
registries. This allows you to push images directly to fly from
the docker cli.
`
		short = "Authenticate docker"
	)

	return command.New("docker", short, long, runDocker,
		command.RequireSession)
}

// ensureDockerConfigDir checks to see if the "${HOME}/.docker" directory exists,
// it creates the dir if it doesn't.
func ensureDockerConfigDir(home string) error {
	dockerDir := filepath.Join(home, ".docker")
	fi, err := os.Stat(dockerDir)
	if err != nil {
		if !os.IsNotExist(err) {
			return err
		}
		// It needs to be readable by Docker, if it gets installed in the
		// future.
		if err := os.Mkdir(dockerDir, 0o755); err != nil {
			return err
		}
	} else if !fi.IsDir() {
		return errors.New("~/.docker is not a dir")
	}
	return nil
}

// addFlyAuthToDockerConfig adds the fly registry to the provided JSON object
// and returns the updated JSON.
//
// The config.json is structured as follows:
//   {
//     "auths": {
//       "registry.fly.io": {
//         "auth": "x:..."
//       }
//     }
//   }
func addFlyAuthToDockerConfig(cfg *config.Config, configJSON []byte) ([]byte, error) {
	var dockerConfig map[string]json.RawMessage
	if len(configJSON) == 0 {
		dockerConfig = make(map[string]json.RawMessage)
	} else if err := json.Unmarshal(configJSON, &dockerConfig); err != nil {
		return nil, err
	}

	var dockerAuthProviders map[string]json.RawMessage
	if a, ok := dockerConfig["auths"]; ok {
		if err := json.Unmarshal(a, &dockerAuthProviders); err != nil {
			return nil, err
		}
	} else {
		dockerAuthProviders = make(map[string]json.RawMessage)
	}

	var flyAuth map[string]interface{}
	if a, ok := dockerAuthProviders[cfg.RegistryHost]; ok {
		if err := json.Unmarshal(a, &flyAuth); err != nil {
			return nil, err
		}
	} else {
		flyAuth = make(map[string]interface{})
	}
	flyAuth["auth"] = base64.URLEncoding.EncodeToString([]byte("x:" + cfg.AccessToken))

	b, err := json.Marshal(flyAuth)
	if err != nil {
		return nil, err
	}
	dockerAuthProviders[cfg.RegistryHost] = b

	b, err = json.Marshal(dockerAuthProviders)
	if err != nil {
		return nil, err
	}

	dockerConfig["auths"] = b

	return json.Marshal(dockerConfig)
}

// configureDockerJSON adds the fly registry to the docker config.json.
func configureDockerJSON(cfg *config.Config) error {
	if runtime.GOOS == "windows" {
		return errors.New("unsuppported")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	if err := ensureDockerConfigDir(home); err != nil {
		return err
	}

	configPath := filepath.Join(home, ".docker", "config.json")
	configJSON, err := os.ReadFile(configPath)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	updatedJSON, err := addFlyAuthToDockerConfig(cfg, configJSON)
	if err != nil {
		return err
	}
	// It needs to be readable by Docker, if it gets installed in the future.
	return os.WriteFile(configPath, updatedJSON, 0o644)
}

func runDocker(ctx context.Context) (err error) {
	cfg := config.FromContext(ctx)
	binary, err := exec.LookPath("docker")
	if err != nil {
		// Try configuring the JSON directly.
		if err := configureDockerJSON(cfg); err == nil {
			return nil
		}
		return fmt.Errorf("docker cli not found - make sure it's installed and try again: %w", err)
	}

	host := cfg.RegistryHost

	var out bytes.Buffer
	cmd := exec.CommandContext(ctx, binary, "login", "--username=x", "--password-stdin", host)
	cmd.Stdout = &out
	cmd.Stderr = &out

	var in io.WriteCloser
	if in, err = cmd.StdinPipe(); err != nil {
		return
	}

	go func() {
		defer in.Close()

		fmt.Fprint(in, cfg.AccessToken)
	}()

	if err = cmd.Start(); err != nil {
		return
	}

	if err = cmd.Wait(); err != nil {
		err = fmt.Errorf("failed authenticating with %s: %v", host, out.String())

		return
	}

	io := iostreams.FromContext(ctx)

	fmt.Fprintf(io.Out, "Authentication successful. You can now tag and push images to %s/{your-app}\n", host)

	return
}
