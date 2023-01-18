package machine

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/flaps"
	"github.com/superfly/flyctl/internal/app"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/command/apps"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/iostreams"
)

func newDestroy() *cobra.Command {
	const (
		short = "Destroy a Fly machine"
		long  = short + "\n"

		usage = "destroy <id>"
	)

	cmd := command.New(usage, short, long, runMachineDestroy,
		command.RequireSession,
		command.LoadAppNameIfPresent,
	)

	cmd.Aliases = []string{"remove", "rm"}

	flag.Add(
		cmd,
		flag.App(),
		flag.AppConfig(),
		flag.Bool{
			Name:        "force",
			Shorthand:   "f",
			Description: "force kill machine if it's running",
		},
	)

	cmd.Args = cobra.MinimumNArgs(1)

	return cmd
}

func runMachineDestroy(ctx context.Context) (err error) {
	var (
		appName   = app.NameFromContext(ctx)
		out       = iostreams.FromContext(ctx).Out
		machineID = flag.FirstArg(ctx)
		input     = api.RemoveMachineInput{
			AppID: app.NameFromContext(ctx),
			ID:    machineID,
			Kill:  flag.GetBool(ctx, "force"),
		}
	)

	app, err := appFromMachineOrName(ctx, machineID, appName)
	if err != nil {
		return err
	}

	ctx, err = apps.BuildContext(ctx, app)
	if err != nil {
		return err
	}

	flapsClient := flaps.FromContext(ctx)

	// check if machine even exists
	current, err := flapsClient.Get(ctx, machineID)
	if err != nil {
		return fmt.Errorf("could not retrieve machine %s", machineID)
	}

	switch current.State {
	case "destroyed":
		return fmt.Errorf("machine %s has already been destroyed", machineID)
	case "started":
		if !input.Kill {
			return fmt.Errorf("machine %s currently started, either stop first or use --force flag", machineID)
		}
	}
	fmt.Fprintf(out, "machine %s was found and is currently in %s state, attempting to destroy...\n", machineID, current.State)

	err = flapsClient.Destroy(ctx, input)
	if err != nil {
		switch {
		case strings.Contains(err.Error(), "not found") && appName != "":
			return fmt.Errorf("could not find machine %s in app %s to destroy", machineID, appName)
		default:
			return fmt.Errorf("could not destroy machine %s: %w", machineID, err)
		}
	}

	// Best effort post-deletion hook.
	runOnDeletionHook(ctx, app, current)

	fmt.Fprintf(out, "%s has been destroyed\n", machineID)

	return
}
