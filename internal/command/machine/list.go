package machine

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/flaps"
	"github.com/superfly/flyctl/internal/appconfig"
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
		command.RequireAppName,
	)

	cmd.Aliases = []string{"ls"}
	cmd.Args = cobra.NoArgs

	flag.Add(
		cmd,
		flag.App(),
		flag.AppConfig(),
		flag.JSONOutput(),
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
		appName = appconfig.NameFromContext(ctx)
		io      = iostreams.FromContext(ctx)
		silence = flag.GetBool(ctx, "quiet")
		cfg     = config.FromContext(ctx)
	)

	flapsClient, err := flaps.NewFromAppName(ctx, appName)
	if err != nil {
		return fmt.Errorf("list of machines could not be retrieved: %w", err)
	}

	machines, err := flapsClient.List(ctx, "")
	if err != nil {
		return fmt.Errorf("machines could not be retrieved")
	}

	if cfg.JSONOutput {
		return render.JSON(io.Out, machines)
	}

	if len(machines) == 0 {
		if !silence {
			fmt.Fprintf(io.Out, "No machines are available on this app %s\n", appName)
		}
		return nil
	}

	rows := [][]string{}

	listOfMachinesLink := io.CreateLink("View them in the UI here", fmt.Sprintf("https://fly.io/apps/%s/machines/", appName))

	if !silence {
		fmt.Fprintf(io.Out, "%d machines have been retrieved from app %s.\n%s\n\n", len(machines), appName, listOfMachinesLink)
	}
	if silence {
		for _, machine := range machines {
			rows = append(rows, []string{machine.ID})
		}
		_ = render.Table(io.Out, "", rows)
	} else {
		for _, machine := range machines {
			var volName string
			if machine.Config != nil && len(machine.Config.Mounts) > 0 {
				volName = machine.Config.Mounts[0].Volume
			}

			appPlatform := ""
			machineProcessGroup := ""
			size := ""

			if machine.Config != nil {
				if platformVersion, ok := machine.Config.Metadata[api.MachineConfigMetadataKeyFlyPlatformVersion]; ok {
					appPlatform = platformVersion
				}

				if processGroup := machine.ProcessGroup(); processGroup != "" {
					machineProcessGroup = processGroup
				}

				if machine.Config.Guest != nil {
					size = fmt.Sprintf("%s:%dMB", machine.Config.Guest.ToSize(), machine.Config.Guest.MemoryMB)
				}
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
				appPlatform,
				machineProcessGroup,
				size,
			})

		}

		_ = render.Table(io.Out, appName, rows, "ID", "Name", "State", "Region", "Image", "IP Address", "Volume", "Created", "Last Updated", "App Platform", "Process Group", "Size")
	}

	return nil
}
