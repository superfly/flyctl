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
	"time"

	"github.com/dustin/go-humanize"
	"github.com/spf13/cobra"
	"github.com/superfly/macaroon"

	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/iostreams"
)

func newDocker() *cobra.Command {
	const (
		long = `Adds registry.fly.io to the Docker daemon's authenticated
registries. This allows you to push images directly to Fly.io from
the Docker CLI. Note that tokens may expire.`
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
		// The permission is 700 as like Docker itself.
		// https://github.com/docker/cli/blob/v23.0.5/cli/config/configfile/file.go#L142
		if err := os.Mkdir(dockerDir, 0o700); err != nil {
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
//
//	{
//	  "auths": {
//	    "registry.fly.io": {
//	      "auth": "x:..."
//	    }
//	  }
//	}
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

	var flyAuth map[string]any
	if a, ok := dockerAuthProviders[cfg.RegistryHost]; ok {
		if err := json.Unmarshal(a, &flyAuth); err != nil {
			return nil, err
		}
	} else {
		flyAuth = make(map[string]any)
	}
	flyAuth["auth"] = base64.URLEncoding.EncodeToString([]byte("x:" + cfg.Tokens.Docker()))

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
	if err := os.WriteFile(configPath, updatedJSON, 0o600); err != nil {
		return err
	}
	// os.WriteFile only applies perm on file creation; Chmod explicitly so
	// the mode is applied on rewrite as well.
	return os.Chmod(configPath, 0o600)
}

func runDocker(ctx context.Context) error {
	cfg := config.FromContext(ctx)
	streams := iostreams.FromContext(ctx)
	binary, err := exec.LookPath("docker")
	if err != nil {
		// Try configuring the JSON directly.
		if err := configureDockerJSON(cfg); err == nil {
			printDockerAuthSuccess(streams.Out, cfg, time.Now())

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

	_, err = fmt.Fprint(in, cfg.Tokens.Docker())
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

	printDockerAuthSuccess(iostreams.FromContext(ctx).Out, cfg, time.Now())

	return nil
}

func printDockerAuthSuccess(out io.Writer, cfg *config.Config, now time.Time) {
	fmt.Fprintf(out, "Authentication successful. You can now tag and push images to %s/{your-app}\n", cfg.RegistryHost)

	if expiration, ok := dockerCredentialExpiration(cfg.Tokens.Docker()); ok {
		fmt.Fprintf(out, "Earliest credential expiration: %s\n", humanize.RelTime(expiration, now, "ago", "from now"))
	} else {
		fmt.Fprintln(out, "Credential expiration: unknown")
	}
}

// dockerCredentialExpiration returns the earliest expiration encoded in the
// credential given to Docker. OAuth tokens and macaroons without validity
// windows do not expose an expiration locally.
func dockerCredentialExpiration(credential string) (time.Time, bool) {
	rawTokens, err := macaroon.Parse(credential)
	if err != nil {
		return time.Time{}, false
	}

	var earliest time.Time
	for _, rawToken := range rawTokens {
		token, err := macaroon.Decode(rawToken)
		if err != nil {
			return time.Time{}, false
		}

		for _, validity := range macaroon.GetCaveats[*macaroon.ValidityWindow](&token.UnsafeCaveats) {
			expiration := time.Unix(validity.NotAfter, 0)
			if earliest.IsZero() || expiration.Before(earliest) {
				earliest = expiration
			}
		}
	}

	return earliest, !earliest.IsZero()
}
