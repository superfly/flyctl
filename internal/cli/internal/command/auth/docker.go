package auth

import (
	"context"
	"fmt"
	"os/exec"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/flyctl"
	"github.com/superfly/flyctl/internal/cli/internal/command"
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

func runDocker(ctx context.Context) error {
	binary, err := exec.LookPath("docker")
	if err != nil {
		return errors.Wrap(err, "docker cli not found - make sure it's installed and try again")
	}

	token := flyctl.GetAPIToken()

	cmd := exec.CommandContext(ctx, binary, "login", "--username=x", "--password-stdin", "registry.fly.io")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() {
		defer stdin.Close()
		fmt.Fprint(stdin, token)
	}()

	if err := cmd.Wait(); err != nil {
		return err
	}

	if !cmd.ProcessState.Success() {
		output, err := cmd.CombinedOutput()
		if err != nil {
			return err
		}
		fmt.Println(output)
		return errors.New("error authenticating with registry.fly.io")
	}

	fmt.Println("Authentication successful. You can now tag and push images to registry.fly.io/{your-app}")

	return nil
}
