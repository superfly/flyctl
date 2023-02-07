package machine

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/flaps"
	"github.com/superfly/flyctl/internal/app"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/render"
	"github.com/superfly/flyctl/iostreams"
)

func newList() *cobra.Command {
	const (
		short = "List Fly machines"
		long  = short + "\n"

		usage = "list"
	)

	cmd := command.New(usage, short, long, runMachineList,
		command.RequireSession,
		command.LoadAppNameIfPresent,
	)

	cmd.Aliases = []string{"ls"}
	cmd.Args = cobra.NoArgs

	flag.Add(
		cmd,
		flag.App(),
		flag.AppConfig(),
		flag.Bool{
			Name:        "quiet",
			Shorthand:   "q",
			Description: "Only list machine ids",
		},
	)

	return cmd
}

func runMachineList(ctx context.Context) (err error) {
	var (
		appName = app.NameFromContext(ctx)
		client  = client.FromContext(ctx).API()
		io      = iostreams.FromContext(ctx)
		silence = flag.GetBool(ctx, "quiet")
		cfg     = config.FromContext(ctx)
	)

	app, err := client.GetAppCompact(ctx, appName)
	if err != nil {
		help := newList().Help()

		if help != nil {
			fmt.Println(help)

		}

		fmt.Println()

		return err
	}
	flapsClient, err := flaps.New(ctx, app)
	if err != nil {
		return fmt.Errorf("list of machines could not be retrieved: %w", err)
	}

	machines, err := flapsClient.List(ctx, "")
	if err != nil {
		return fmt.Errorf("machines could not be retrieved")
	}

	if len(machines) == 0 {
		fmt.Fprintf(io.Out, "No machines are available on this app %s\n", appName)
		return nil
	}

	if cfg.JSONOutput {
		return render.JSON(io.Out, machines)
	}

	rows := [][]string{}

	listOfMachinesLink := io.CreateLink("View them in the UI here", fmt.Sprintf("https://fly.io/apps/%s/machines/", appName))
	fmt.Fprintf(io.Out, "%d machines have been retrieved from app %s.\n%s\n\n", len(machines), appName, listOfMachinesLink)
	if silence {
		for _, machine := range machines {
			rows = append(rows, []string{machine.ID})
		}
		_ = render.Table(io.Out, appName, rows, "ID")
	} else {
		for _, machine := range machines {
			var volName string
			if machine.Config != nil && len(machine.Config.Mounts) > 0 {
				volName = machine.Config.Mounts[0].Volume
			}

			rows = append(rows, []string{
				machine.ID,
				machine.Name,
				machine.State,
				machine.Region,
				machine.ImageRefWithVersion(),
				machine.PrivateIP,
				volName,
				machine.CreatedAt,
				machine.UpdatedAt,
			})
		}

		_ = render.Table(io.Out, appName, rows, "ID", "Name", "State", "Region", "Image", "IP Address", "Volume", "Created", "Last Updated")
	}
	return nil
}
