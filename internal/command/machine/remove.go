package machine

import (
	"context"

	"fmt"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/flaps"
	"github.com/superfly/flyctl/internal/app"
	"github.com/superfly/flyctl/internal/client"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/iostreams"
)

func newRemove() *cobra.Command {
	const (
		short = "Remove a Fly machine"
		long  = short + "\n"

		usage = "remove <id>"
	)

	cmd := command.New(usage, short, long, runMachineRemove,
		command.RequireSession,
		command.LoadAppNameIfPresent,
	)

	cmd.Aliases = []string{"rm"}

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

func runMachineRemove(ctx context.Context) (err error) {
	var (
		appName   = app.NameFromContext(ctx)
		client    = client.FromContext(ctx).API()
		out       = iostreams.FromContext(ctx).Out
		machineID = flag.FirstArg(ctx)
		input     = api.RemoveMachineInput{
			AppID: app.NameFromContext(ctx),
			ID:    machineID,
			Kill:  flag.GetBool(ctx, "force"),
		}
	)

	if appName == "" {
		return fmt.Errorf("app was not found")
	}

	app, err := client.GetAppCompact(ctx, appName)
	if err != nil {
		return err
	}

	flapsClient, err := flaps.New(ctx, app)
	if err != nil {
		return fmt.Errorf("could not make flaps client: %w", err)
	}

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
		return fmt.Errorf("could not destroy machine %s: %w", machineID, err)
	}

	fmt.Fprintf(out, "%s has been destroyed\n", machineID)

	return
}
