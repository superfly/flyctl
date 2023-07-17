package volumes

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/flaps"
	"github.com/superfly/flyctl/iostreams"

	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/render"
)

func newShow() (cmd *cobra.Command) {
	const (
		long = `Show details of an app's volume. Requires the volume's ID
number to operate. This can be found through the volumes list command`

		short = "Show details of an app's volume"
	)

	cmd = command.New("show <id>", short, long, runShow,
		command.RequireSession,
	)
	cmd.Args = cobra.ExactArgs(1)

	flag.Add(cmd, flag.JSONOutput())
	return
}

func runShow(ctx context.Context) error {
	cfg := config.FromContext(ctx)
	appName := appconfig.NameFromContext(ctx)
	flapsClient, err := flaps.NewFromAppName(ctx, appName)
	if err != nil {
		return fmt.Errorf("could not create flaps client: %w", err)
	}
	ctx = flaps.NewContext(ctx, flapsClient)

	volumeID := flag.FirstArg(ctx)

	volume, err := flapsClient.GetVolume(ctx, volumeID)
	if err != nil {
		return fmt.Errorf("failed retrieving volume: %w", err)
	}

	out := iostreams.FromContext(ctx).Out

	if cfg.JSONOutput {
		return render.JSON(out, volume)
	}

	return printVolume(out, volume, appName)
}
