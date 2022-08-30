package volumes

import (
	"context"
	"fmt"
	"strconv"

	"github.com/dustin/go-humanize"
	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/iostreams"

	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/internal/app"
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

	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
	)

	return cmd
}

func runList(ctx context.Context) error {
	cfg := config.FromContext(ctx)
	client := client.FromContext(ctx).API()

	appName := app.NameFromContext(ctx)

	volumes, err := client.GetVolumes(ctx, appName)
	if err != nil {
		return fmt.Errorf("failed retrieving volumes: %w", err)
	}

	out := iostreams.FromContext(ctx).Out

	if cfg.JSONOutput {
		return render.JSON(out, volumes)
	}

	rows := make([][]string, 0, len(volumes))
	for _, volume := range volumes {
		var attachedAllocID string

		if volume.AttachedAllocation != nil {
			attachedAllocID = volume.AttachedAllocation.IDShort

			if volume.AttachedAllocation.TaskName != "app" {
				attachedAllocID = fmt.Sprintf("%s (%s)", volume.AttachedAllocation.IDShort, volume.AttachedAllocation.TaskName)
			}
		}

		rows = append(rows, []string{
			volume.ID,
			volume.State,
			volume.Name,
			strconv.Itoa(volume.SizeGb) + "GB",
			volume.Region,
			volume.Host.ID,
			fmt.Sprint(volume.Encrypted),
			attachedAllocID,
			humanize.Time(volume.CreatedAt),
		})

	}

	return render.Table(out, "", rows, "ID", "State", "Name", "Size", "Region", "Zone", "Encrypted", "Attached VM", "Created At")
}
