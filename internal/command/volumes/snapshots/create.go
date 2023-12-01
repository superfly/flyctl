package snapshots

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/flaps"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
)

func newCreate() *cobra.Command {
	const (
		short = "Snapshot a volume"
		long  = "Snapshot a volume\n"
		usage = "create <volume-id>"
	)

	cmd := command.New(usage, short, long, create, command.RequireSession)

	cmd.Aliases = []string{"create"}

	cmd.Args = cobra.ExactArgs(1)

	flag.Add(cmd, flag.JSONOutput())
	return cmd
}

func create(ctx context.Context) error {
	var client = client.FromContext(ctx).API()

	volumeId := flag.FirstArg(ctx)

	appName := appconfig.NameFromContext(ctx)
	if appName == "" {
		n, err := client.GetAppNameFromVolume(ctx, volumeId)
		if err != nil {
			return err
		}
		appName = *n
	}

	flapsClient, err := flaps.NewFromAppName(ctx, appName)
	if err != nil {
		return err
	}

	err = flapsClient.CreateVolumeSnapshot(ctx, volumeId)
	if err != nil {
		return err
	}

	fmt.Printf("Scheduled to snapshot volume %s\n", volumeId)

	return nil
}
