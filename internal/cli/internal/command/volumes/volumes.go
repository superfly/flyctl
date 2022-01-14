package volumes

import (
	"bytes"
	"fmt"
	"io"
	"time"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/api"

	"github.com/superfly/flyctl/internal/cli/internal/command"
	"github.com/superfly/flyctl/internal/cli/internal/command/volumes/snapshots"
)

func New() *cobra.Command {
	const (
		long = "Commands for managing Fly Volumes associated with an application"

		short = "Volume management commands"

		usage = "volumes <command>"
	)

	cmd := command.New(usage, short, long, nil)

	cmd.Aliases = []string{"vol"}

	cmd.AddCommand(
		newCreate(),
		newList(),
		newDelete(),
		newShow(),
		snapshots.New(),
	)

	return cmd
}

func printVolume(w io.Writer, vol *api.Volume) error {
	var buf bytes.Buffer

	fmt.Fprintf(&buf, "%10s: %s\n", "ID", vol.ID)
	fmt.Fprintf(&buf, "%10s: %s\n", "Name", vol.Name)
	fmt.Fprintf(&buf, "%10s: %s\n", "App", vol.App.Name)
	fmt.Fprintf(&buf, "%10s: %s\n", "Region", vol.Region)
	fmt.Fprintf(&buf, "%10s: %s\n", "Zone", vol.Host.ID)
	fmt.Fprintf(&buf, "%10s: %d\n", "Size GB", vol.SizeGb)
	fmt.Fprintf(&buf, "%10s: %t\n", "Encrypted", vol.Encrypted)
	fmt.Fprintf(&buf, "%10s: %s\n", "Created at", vol.CreatedAt.Format(time.RFC822))

	_, err := buf.WriteTo(w)

	return err
}
