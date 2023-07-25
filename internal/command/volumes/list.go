package volumes

import (
	"context"
	"fmt"
	"strconv"

	"github.com/dustin/go-humanize"
	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/flaps"
	"github.com/superfly/flyctl/iostreams"

	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/render"
)

func newList() *cobra.Command {
	const (
		long = "List all the volumes associated with this application."

		short = "List the volumes for app"
	)

	cmd := command.New("list", short, long, runList,
		command.RequireSession,
		command.RequireAppName,
	)

	cmd.Aliases = []string{"ls"}

	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
	)

	flag.Add(cmd, flag.JSONOutput())
	return cmd
}

func runList(ctx context.Context) error {
	cfg := config.FromContext(ctx)
	apiClient := client.FromContext(ctx).API()

	appName := appconfig.NameFromContext(ctx)

	app, err := apiClient.GetApp(ctx, appName)
	if err != nil {
		return err
	}

	flapsClient, err := flaps.NewFromAppName(ctx, appName)
	if err != nil {
		return err
	}

	volumes, err := flapsClient.GetVolumes(ctx)
	if err != nil {
		return fmt.Errorf("failed retrieving volumes: %w", err)
	}

	out := iostreams.FromContext(ctx).Out

	if cfg.JSONOutput {
		return render.JSON(out, volumes)
	}

	rows := make([][]string, 0, len(volumes))
	for _, volume := range volumes {
		var attachedVMID string

		if app.PlatformVersion == "machines" {
			if volume.AttachedMachine != nil {
				attachedVMID = *volume.AttachedMachine
			}
		} else {
			names, err := apiClient.GetAllocationTaskNames(ctx, appName)
			if err != nil {
				return err
			}

			if volume.AttachedAllocation != nil {
				attachedVMID = *volume.AttachedAllocation

				taskName, ok := names[*volume.AttachedAllocation]

				if ok && taskName != "app" {
					attachedVMID = fmt.Sprintf("%s (%s)", *volume.AttachedAllocation, taskName)
				}
			}
		}

		rows = append(rows, []string{
			volume.ID,
			volume.State,
			volume.Name,
			strconv.Itoa(volume.SizeGb) + "GB",
			volume.Region,
			volume.Zone,
			fmt.Sprint(volume.Encrypted),
			attachedVMID,
			humanize.Time(volume.CreatedAt),
		})
	}

	return render.Table(out, "", rows, "ID", "State", "Name", "Size", "Region", "Zone", "Encrypted", "Attached VM", "Created At")
}
