package auth

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/internal/cli/internal/command"
	"github.com/superfly/flyctl/internal/cli/internal/config"
	"github.com/superfly/flyctl/pkg/iostreams"
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

func runDocker(ctx context.Context) (err error) {
	binary, err := exec.LookPath("docker")
	if err != nil {
		return errors.Wrap(err, "docker cli not found - make sure it's installed and try again")
	}

	cfg := config.FromContext(ctx)
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
