package snapshots

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/flyutil"
)

func newCreate() *cobra.Command {
	const (
		short = "Create a volume snapshot."
		long  = "Snapshot a volume\n"
		usage = "create <volume id>"
	)

	cmd := command.New(usage, short, long, create, command.RequireSession)
	cmd.Args = cobra.ExactArgs(1)

	flag.Add(cmd, flag.JSONOutput())
	return cmd
}

func create(ctx context.Context) error {
	client := flyutil.ClientFromContext(ctx)

	volumeId := flag.FirstArg(ctx)

	appName := appconfig.NameFromContext(ctx)
	if appName == "" {
		n, err := client.GetAppNameFromVolume(ctx, volumeId)
		if err != nil {
			return err
		}
		appName = *n
	}

	flapsClient, err := flapsutil.NewClientWithOptions(ctx, flaps.NewClientOpts{
		AppName: appName,
	})
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
