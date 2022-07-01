package machine

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/flaps"
	"github.com/superfly/flyctl/internal/app"
	"github.com/superfly/flyctl/internal/client"
	"github.com/superfly/flyctl/internal/command"
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
	)

	if appName == "" {
		return fmt.Errorf("app is not found")
	}
	app, err := client.GetAppCompact(ctx, appName)
	if err != nil {
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

	rows := [][]string{}

	listOfMachinesLink := io.CreateLink("View them in the UI here", fmt.Sprintf("https://fly.io/apps/%s/machines/", appName))
	fmt.Fprintf(io.Out, "%d machines have been retrieved.\n%s\n\n", len(machines), listOfMachinesLink)
	if silence {
		for _, machine := range machines {
			rows = append(rows, []string{machine.ID})
		}
		_ = render.Table(io.Out, appName, rows, "ID")
	} else {
		for _, machine := range machines {
			rows = append(rows, []string{
				machine.ID,
				fmt.Sprintf("%s:%s", machine.ImageRef.Repository, machine.ImageRef.Tag),
				machine.CreatedAt,
				machine.UpdatedAt,
				machine.State,
				machine.Region,
				machine.Name,
				machine.PrivateIP,
			})
		}

		_ = render.Table(io.Out, appName, rows, "ID", "Image", "Created", "Last Updated", "State", "Region", "Name", "IP Address")
	}
	return nil
}
