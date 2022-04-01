package machine

import (
	"context"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/cli/internal/app"
	"github.com/superfly/flyctl/internal/cli/internal/command"
	"github.com/superfly/flyctl/internal/cli/internal/render"
	"github.com/superfly/flyctl/internal/client"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag"
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
		cfg    = config.FromContext(ctx)
		client = client.FromContext(ctx).API()
	)

	var (
		appName   = app.NameFromContext(ctx)
		machineID = flag.FirstArg(ctx)
	)

	machine, err := client.GetMachine(ctx, appName, machineID)
	if err != nil {
		return err
	}

	if cfg.JSONOutput {
		return render.JSON(io.Out, machine)
	}

	if err := render.MachineStatus(io.Out, machine); err != nil {
		return err
	}

	// render ips
	if err := render.MachineIPs(io.Out, machine.IPs.Nodes...); err != nil {
		return err
	}

	// render machine events
	if err := render.MachineEvents(io.Out, machine.Events.Nodes...); err != nil {
		return err
	}

	return nil
}
