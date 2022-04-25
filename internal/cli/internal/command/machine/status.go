package machine

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/internal/cli/internal/app"
	"github.com/superfly/flyctl/internal/cli/internal/command"
	"github.com/superfly/flyctl/internal/client"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/pkg/flaps"
	"github.com/superfly/flyctl/pkg/iostreams"
)

func newStatus() *cobra.Command {
	const (
		short = "Show current status of a running machine"
		long  = short + "\n"

		usage = "status <id>"
	)

	cmd := command.New(usage, short, long, runMachineStatus,
		command.RequireSession,
		command.RequireAppName,
	)

	cmd.Args = cobra.ExactArgs(1)

	flag.Add(
		cmd,
		flag.App(),
		flag.AppConfig(),
	)

	return cmd
}

func runMachineStatus(ctx context.Context) error {
	var (
		io     = iostreams.FromContext(ctx)
		client = client.FromContext(ctx).API()
	)

	var (
		appName   = app.NameFromContext(ctx)
		machineID = flag.FirstArg(ctx)
	)

	// flaps client
	if appName == "" {
		return fmt.Errorf("app is not found")
	}
	app, err := client.GetApp(ctx, appName)
	if err != nil {
		return err
	}
	flapsClient, err := flaps.New(ctx, app)
	if err != nil {
		return fmt.Errorf("could not make flaps client: %w", err)
	}

	machineBody, err := flapsClient.Get(ctx, machineID)
	if err != nil {
		return fmt.Errorf("machine %s could not be retrieved", machineID)
	}
	var machine api.V1Machine
	err = json.Unmarshal(machineBody, &machine)
	if err != nil {
		return fmt.Errorf("machine %s could not be retrieved", machineID)
	}

	fmt.Fprintf(io.Out, "Success! A machine has been retrieved\n")
	fmt.Fprintf(io.Out, " Machine ID: %s\n", machine.ID)
	fmt.Fprintf(io.Out, " Instance ID: %s\n", machine.InstanceID)
	fmt.Fprintf(io.Out, " State: %s\n", machine.State)
	fmt.Fprintf(io.Out, " Region: %s\n", machine.Region)
	fmt.Fprintf(io.Out, " Image: %s\n", machine.Config.Image)

	return nil
}
