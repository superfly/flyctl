package machine

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/flaps"
	"github.com/superfly/flyctl/internal/app"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/iostreams"
)

func newClone() *cobra.Command {
	const (
		short = "Clone a Fly machine"
		long  = short + "\n"

		usage = "clone <id>"
	)

	cmd := command.New(usage, short, long, runMachineClone,
		command.RequireSession,
		command.LoadAppNameIfPresent,
	)

	cmd.Args = cobra.MaximumNArgs(1)

	flag.Add(
		cmd,
		flag.App(),
		flag.AppConfig(),
		flag.String{
			Name:        "region",
			Description: "Target region for the new machine",
		},
		flag.String{
			Name:        "name",
			Description: "Optional name for the new machine",
		},
	)

	return cmd
}

func runMachineClone(ctx context.Context) (err error) {
	var (
		args    = flag.Args(ctx)
		out     = iostreams.FromContext(ctx).Out
		appName = app.NameFromContext(ctx)
		client  = client.FromContext(ctx).API()
	)

	app, err := client.GetAppCompact(ctx, appName)
	if err != nil {
		return err
	}

	flapsClient, err := flaps.New(ctx, app)
	if err != nil {
		return fmt.Errorf("could not make flaps client: %w", err)
	}

	var source *api.Machine

	if len(args) > 0 {
		source, err = flapsClient.Get(ctx, args[0])
		if err != nil {
			return err
		}
	} else {
		fmt.Fprintf(out, "No machine ID specified, so picking one at random\n")
		machines, err := flapsClient.List(ctx, "started")
		if err != nil {
			return err
		}

		source, err = flapsClient.Get(ctx, machines[0].ID)

		if err != nil {
			return err
		}

		fmt.Fprintf(out, "Picked %s for cloning\n", source.ID)
	}

	region := flag.GetString(ctx, "region")

	if region == "" {
		region = source.Region
	}

	// Ensure attached volumes are not copied to the clone
	source.Config.Mounts = nil

	input := api.LaunchMachineInput{
		AppID:  app.Name,
		Name:   flag.GetString(ctx, "name"),
		Region: region,
		Config: source.Config,
	}

	launchedMachine, err := flapsClient.Launch(ctx, input)
	if err != nil {
		return err
	}

	err = flapsClient.Wait(ctx, launchedMachine, "started")

	if err != nil {
		return err
	}

	fmt.Fprintf(out, "Machine %s was cloned to %s\n", source.ID, launchedMachine.ID)
	return
}
