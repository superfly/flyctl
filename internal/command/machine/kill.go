package machine

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/internal/app"
	"github.com/superfly/flyctl/internal/client"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/pkg/flaps"
	"github.com/superfly/flyctl/pkg/iostreams"
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
		client    = client.FromContext(ctx).API()
		machineID = flag.FirstArg(ctx)
		io        = iostreams.FromContext(ctx)
	)

	if appName == "" {
		return fmt.Errorf("app was not found")
	}

	app, err := client.GetApp(ctx, appName)
	if err != nil {
		return err
	}

	flapsClient, err := flaps.New(ctx, app)
	if err != nil {
		return fmt.Errorf("could not make flaps client: %w", err)
	}

	// check if machine even exists //
	machineBody := api.V1Machine{}
	currentMachine, err := flapsClient.Get(ctx, machineID)
	if err != nil {
		return fmt.Errorf("could not retrieve machine %s", machineID)
	}

	if err := json.Unmarshal(currentMachine, &machineBody); err != nil {
		return fmt.Errorf("could not read machine body %s: %w", machineID, err)
	}

	if machineBody.State == "destroyed" {
		return fmt.Errorf("machine %s has already been destroyed", machineID)
	}
	fmt.Fprintf(io.Out, "machine %s was found and is currently in a %s state, attempting to kill...\n", machineID, machineBody.State)

	_, err = flapsClient.Kill(ctx, machineID)
	if err != nil {
		return fmt.Errorf("could not kill machine %s: %w", machineID, err)
	}

	fmt.Fprintln(io.Out, "kill signal has been sent")

	return nil
}
