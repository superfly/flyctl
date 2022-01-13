package volumes

import (
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
	)

	cmd := command.New("volumes [type] <name> [flags]", short, long, nil)

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

func printVolume(writer io.Writer, vol *api.Volume) {
	fmt.Printf("%10s: %s\n", "ID", vol.ID)
	fmt.Printf("%10s: %s\n", "Name", vol.Name)
	fmt.Printf("%10s: %s\n", "App", vol.App.Name)
	fmt.Printf("%10s: %s\n", "Region", vol.Region)
	fmt.Printf("%10s: %s\n", "Zone", vol.Host.ID)
	fmt.Printf("%10s: %d\n", "Size GB", vol.SizeGb)
	fmt.Printf("%10s: %t\n", "Encrypted", vol.Encrypted)
	fmt.Printf("%10s: %s\n", "Created at", vol.CreatedAt.Format(time.RFC822))
}
