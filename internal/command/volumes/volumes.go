package volumes

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"time"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/flaps"

	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/command/volumes/snapshots"
)

func New() *cobra.Command {
	const (
		long = "Commands for managing Fly Volumes associated with an application"

		short = "Volume management commands"

		usage = "volumes <command>"
	)

	cmd := command.New(usage, short, long, nil)

	cmd.Aliases = []string{"volume", "vol"}

	cmd.AddCommand(
		newCreate(),
		newList(),
		newDestroy(),
		newExtend(),
		newShow(),
		newFork(),
		snapshots.New(),
	)

	return cmd
}

func printVolume(w io.Writer, vol *api.Volume, appName string) error {
	var buf bytes.Buffer

	fmt.Fprintf(&buf, "%10s: %s\n", "ID", vol.ID)
	fmt.Fprintf(&buf, "%10s: %s\n", "Name", vol.Name)
	fmt.Fprintf(&buf, "%10s: %s\n", "App", appName)
	fmt.Fprintf(&buf, "%10s: %s\n", "Region", vol.Region)
	fmt.Fprintf(&buf, "%10s: %s\n", "Zone", vol.Zone)
	fmt.Fprintf(&buf, "%10s: %d\n", "Size GB", vol.SizeGb)
	fmt.Fprintf(&buf, "%10s: %t\n", "Encrypted", vol.Encrypted)
	fmt.Fprintf(&buf, "%10s: %s\n", "Created at", vol.CreatedAt.Format(time.RFC822))

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
