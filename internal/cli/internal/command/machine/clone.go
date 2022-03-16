package machine

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/api"

	"github.com/superfly/flyctl/cmd/presenters"
	"github.com/superfly/flyctl/helpers"

	"github.com/superfly/flyctl/pkg/iostreams"
	"github.com/superfly/flyctl/pkg/logs"

	"github.com/superfly/flyctl/internal/cli/internal/app"
	"github.com/superfly/flyctl/internal/cli/internal/command"
	"github.com/superfly/flyctl/internal/cli/internal/config"
	"github.com/superfly/flyctl/internal/cli/internal/flag"
	"github.com/superfly/flyctl/internal/cli/internal/prompt"
	"github.com/superfly/flyctl/internal/client"
	"github.com/superfly/flyctl/internal/logger"
)

func newClone() *cobra.Command {
	const (
		short = "Clones a Fly Machine"
		long  = short + "\n"

		usage = "clone <id>"
	)

	cmd := command.New(usage, short, long, runMachineClone,
		command.RequireSession,
		command.RequireAppName,
	)

	cmd.Args = cobra.ExactArgs(1)

	flag.Add(
		cmd,
		flag.App(),
		flag.AppConfig(),
		flag.Region(),
		flag.String{
			Name:        "name",
			Shorthand:   "n",
			Description: "The name of the new machine",
		},
		flag.String{
			Name:        "organization",
			Shorthand:   "o",
			Description: "Target organization",
		},
		flag.Bool{
			Name:        "detach",
			Shorthand:   "d",
			Description: "Detach from the machine's logs",
		},
	)

	return cmd
}

func runMachineClone(ctx context.Context) error {

	var (
		appName   = app.NameFromContext(ctx)
		machineID = flag.FirstArg(ctx)
		name      = flag.GetString(ctx, "name")
		out       = iostreams.FromContext(ctx).Out
		cfg       = config.FromContext(ctx)
		client    = client.FromContext(ctx).API()
		logger    = logger.FromContext(ctx)
	)

	region, err := prompt.Region(ctx)
	if err != nil {
		return fmt.Errorf("could not get region: %w", err)
	}

	org, err := prompt.Org(ctx, nil)
	if err != nil {
		return fmt.Errorf("could not get organization: %w", err)
	}

	machine, err := client.GetMachine(ctx, appName, machineID)
	if err != nil {
		return fmt.Errorf("failed to resolve machine with id %s: %w", machineID, err)
	}

	if len(machine.Config.Mounts) > 0 {
		volumeHash, err := helpers.RandString(5)
		if err != nil {
			return fmt.Errorf("failed to generate volume hash: %w", err)
		}
		// This copies the existing Volume spec and just renames it.
		mount := machine.Config.Mounts[0]
		mount.Volume = fmt.Sprintf("data_%s", volumeHash)
		machine.Config.Mounts = []api.MachineMount{mount}
	}

	input := api.LaunchMachineInput{
		AppID:   appName,
		Name:    name,
		OrgSlug: org.ID,
		Region:  region.Code,
		Config:  &machine.Config,
	}

	machine, _, err = client.LaunchMachine(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to launch machine: %w", err)
	}

	if flag.GetBool(ctx, "detach") {
		fmt.Fprintln(out, machine.ID)
		return nil
	}

	opts := &logs.LogOptions{
		AppName: appName,
		VMID:    machine.ID,
	}

	stream, err := logs.NewNatsStream(ctx, client, opts)

	if err != nil {
		logger.Debugf("could not connect to wireguard tunnel, err: %v\n", err)
		logger.Debug("Falling back to log polling...")

		stream, err = logs.NewPollingStream(ctx, client, opts)
		if err != nil {
			return fmt.Errorf("failed to get machine logs: %w", err)
		}
	}

	presenter := presenters.LogPresenter{}
	entries := stream.Stream(ctx, opts)

	for {
		select {
		case <-ctx.Done():
			return stream.Err()
		case entry := <-entries:
			presenter.FPrint(out, cfg.JSONOutput, entry)
		}
	}
}
