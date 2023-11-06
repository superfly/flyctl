package volumes

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"strconv"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/flaps"
	"github.com/superfly/flyctl/iostreams"

	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/command/volumes/lsvd"
	"github.com/superfly/flyctl/internal/command/volumes/snapshots"
	"github.com/superfly/flyctl/internal/prompt"
	"github.com/superfly/flyctl/internal/render"
)

func New() *cobra.Command {
	const (
		short = "Manage Fly Volumes."

		long = short

		usage = "volumes <command>"
	)

	cmd := command.New(usage, short, long, nil)

	cmd.Aliases = []string{"volume", "vol"}

	cmd.AddCommand(
		newCreate(),
		newUpdate(),
		newList(),
		newDestroy(),
		newExtend(),
		newShow(),
		newFork(),
		lsvd.New(),
		snapshots.New(),
	)

	return cmd
}

func printVolume(w io.Writer, vol *api.Volume, appName string) error {
	var buf bytes.Buffer

	fmt.Fprintf(&buf, "%20s: %s\n", "ID", vol.ID)
	fmt.Fprintf(&buf, "%20s: %s\n", "Name", vol.Name)
	fmt.Fprintf(&buf, "%20s: %s\n", "App", appName)
	fmt.Fprintf(&buf, "%20s: %s\n", "Region", vol.Region)
	fmt.Fprintf(&buf, "%20s: %s\n", "Zone", vol.Zone)
	fmt.Fprintf(&buf, "%20s: %d\n", "Size GB", vol.SizeGb)
	fmt.Fprintf(&buf, "%20s: %t\n", "Encrypted", vol.Encrypted)
	fmt.Fprintf(&buf, "%20s: %s\n", "Created at", vol.CreatedAt.Format(time.RFC822))
	fmt.Fprintf(&buf, "%20s: %d\n", "Snapshot retention", vol.SnapshotRetention)

	_, err := buf.WriteTo(w)

	return err
}

func countVolumesMatchingName(ctx context.Context, volumeName string) (int32, error) {
	var (
		volumes []api.Volume
		err     error

		flapsClient = flaps.FromContext(ctx)
	)

	if volumes, err = flapsClient.GetVolumes(ctx); err != nil {
		return 0, err
	}

	var matches int32
	for _, volume := range volumes {
		if volume.Name == volumeName {
			matches++
		}
	}

	return matches, nil
}

func renderTable(ctx context.Context, volumes []api.Volume, app *api.App, out io.Writer) error {
	apiClient := client.FromContext(ctx).API()
	rows := make([][]string, 0, len(volumes))
	for _, volume := range volumes {
		var attachedVMID string

		if app.PlatformVersion == "machines" {
			if volume.AttachedMachine != nil {
				attachedVMID = *volume.AttachedMachine
			}
		} else {
			names, err := apiClient.GetAllocationTaskNames(ctx, app.Name)
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

func selectVolume(ctx context.Context, flapsClient *flaps.Client, app *api.App) (*api.Volume, error) {
	if !iostreams.FromContext(ctx).IsInteractive() {
		return nil, fmt.Errorf("volume ID must be specified when not running interactively")
	}
	volumes, err := flapsClient.GetVolumes(ctx)
	if err != nil {
		return nil, err
	}
	if len(volumes) == 0 {
		return nil, fmt.Errorf("no volumes found in app '%s'", app.Name)
	}
	out := new(bytes.Buffer)
	err = renderTable(ctx, volumes, app, out)
	if err != nil {
		return nil, err
	}
	volumeLines := make([]string, 0)
	scanner := bufio.NewScanner(out)
	title := ""
	for scanner.Scan() {
		text := scanner.Text()
		if text == "" {
			continue
		}
		if title == "" {
			title = text
			continue
		}
		volumeLines = append(volumeLines, text)
	}
	selected := 0
	err = prompt.Select(ctx, &selected, title+"\nSelect volume:", "", volumeLines...)
	if err != nil {
		return nil, fmt.Errorf("selecting volume: %w", err)
	}
	return &volumes[selected], nil
}
