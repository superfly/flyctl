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

func newPodman() *cobra.Command {
	const (
		long = `Adds registry.fly.io to the Podman authenticated
registries. This allows you to push images directly to fly from
the podman cli.
`
		short = "Authenticate podman"
	)

	return command.New("podman", short, long, runPodman,
		command.RequireSession)
}

// ensurePodmanConfigDir checks to see if the "${HOME}/.containers/" directory exists,
// it creates the dir if it doesn't.
func ensurePodmanConfigDir(home string) error {
	podmanDir := filepath.Join(home, ".config/containers")
	fi, err := os.Stat(podmanDir)
	if err != nil {
		if !os.IsNotExist(err) {
			return err
		}
		// Ensures that the directory has the correct file permissions
		if err := os.Mkdir(podmanDir, 0o700); err != nil {
			return err
		}
	} else if !fi.IsDir() {
		return errors.New("~/.config/containers is not a dir")
	}
	return nil
}

// addFlyAuthToPodmanConfig adds the fly registry to the provided JSON object
// and returns the updated JSON.
//
// The config.json is structured as follows:
//
//	{
//	  "auths": {
//	    "registry.fly.io": {
//	      "auth": "x:..."
//	    }
//	  }
//	}
func addFlyAuthToPodmanConfig(cfg *config.Config, configJSON []byte) ([]byte, error) {
	var podmanConfig map[string]json.RawMessage
	if len(configJSON) == 0 {
		podmanConfig = make(map[string]json.RawMessage)
	} else if err := json.Unmarshal(configJSON, &podmanConfig); err != nil {
		return nil, err
	}

	var podmanAuthProviders map[string]json.RawMessage
	if a, ok := podmanConfig["auths"]; ok {
		if err := json.Unmarshal(a, &podmanAuthProviders); err != nil {
			return nil, err
		}
	} else {
		podmanAuthProviders = make(map[string]json.RawMessage)
	}

	var flyAuth map[string]interface{}
	if a, ok := podmanAuthProviders[cfg.RegistryHost]; ok {
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
	podmanAuthProviders[cfg.RegistryHost] = b

	b, err = json.Marshal(podmanAuthProviders)
	if err != nil {
		return nil, err
	}

	podmanConfig["auths"] = b

	return json.Marshal(podmanConfig)
}

// configurePodmanJSON adds the fly registry to the podman config.json.
func configurePodmanJSON(cfg *config.Config) error {
	if runtime.GOOS == "windows" {
		return errors.New("unsuppported")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	if err := ensurePodmanConfigDir(home); err != nil {
		return err
	}

	configPath := filepath.Join(home, ".config/containers", "auth.json")
	configJSON, err := os.ReadFile(configPath)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	updatedJSON, err := addFlyAuthToPodmanConfig(cfg, configJSON)
	if err != nil {
		return err
	}
	// It needs to be readable by Podman, if it gets installed in the future.
	return os.WriteFile(configPath, updatedJSON, 0o644)
}

func runPodman(ctx context.Context) error {
	cfg := config.FromContext(ctx)
	binary, err := exec.LookPath("podman")
	if err != nil {
		// Try configuring the JSON directly.
		if err := configurePodmanJSON(cfg); err == nil {
			return nil
		}
		return fmt.Errorf("podman cli not found - make sure it's installed and try again: %w", err)
	}

	host := cfg.RegistryHost

	var out bytes.Buffer
	cmd := exec.CommandContext(ctx, binary, "login", "--username=x", "--password-stdin", host)
	cmd.Stdout = &out
	cmd.Stderr = &out

	var in io.WriteCloser
	if in, err = cmd.StdinPipe(); err != nil {
		return err
	}
	// This defer is for early-returns before successfully writing to the stream, hence safe.
	defer func() {
		if in != nil {
			in.Close() // skipcq: GO-S2307
		}
	}()

	if err = cmd.Start(); err != nil {
		return err
	}

	_, err = fmt.Fprint(in, cfg.AccessToken)
	if err != nil {
		return err
	}

	err = in.Close()
	in = nil // Prevent the deferred function from double-closing
	if err != nil {
		return err
	}

	if err = cmd.Wait(); err != nil {
		return fmt.Errorf("failed authenticating with %s: %v", host, out.String())
	}

	io := iostreams.FromContext(ctx)

	fmt.Fprintf(io.Out, "Authentication successful. You can now tag and push images to %s/{your-app}\n", host)

	return nil
}
