package machine

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/flaps"
	"github.com/superfly/flyctl/internal/app"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/iostreams"
)

func newKill() *cobra.Command {
	const (
		short = "Kill (SIGKILL) a Fly machine"
		long  = short + "\n"

		usage = "kill <id>"
	)

	cmd := command.New(usage, short, long, runMachineKill,
		command.RequireSession,
		command.LoadAppNameIfPresent,
	)

	cmd.Args = cobra.ExactArgs(1)

	flag.Add(
		cmd,
		flag.App(),
		flag.AppConfig(),
	)

	return cmd
}

func runMachineKill(ctx context.Context) (err error) {
	var (
		appName   = app.NameFromContext(ctx)
		machineID = flag.FirstArg(ctx)
		io        = iostreams.FromContext(ctx)
	)

	app, err := appFromMachineOrName(ctx, machineID, appName)

	if err != nil {
		return err
	}

	flapsClient, err := flaps.New(ctx, app)
	if err != nil {
		return fmt.Errorf("could not make flaps client: %w", err)
	}

	current, err := flapsClient.Get(ctx, machineID)
	if err != nil {
		return fmt.Errorf("could not retrieve machine %s", machineID)
	}

	if current.State == "destroyed" {
		return fmt.Errorf("machine %s has already been destroyed", machineID)
	}
	fmt.Fprintf(io.Out, "machine %s was found and is currently in a %s state, attempting to kill...\n", machineID, current.State)

	err = flapsClient.Kill(ctx, machineID)
	if err != nil {
		return fmt.Errorf("could not kill machine %s: %w", machineID, err)
	}

	fmt.Fprintln(io.Out, "kill signal has been sent")

	return nil
}
