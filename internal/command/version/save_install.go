package version

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/internal/cache"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/state"
	"github.com/superfly/flyctl/internal/update"
	"github.com/superfly/flyctl/iostreams"
)

func newSaveInstall() *cobra.Command {
	cmd := command.New(
		"save-install",
		"save-install",
		"Save installation configuration",
		runSaveInstall)

	flag.Add(cmd,
		flag.String{
			Name:        "channel",
			Description: "Channel to use for updates",
		},
	)
	flag.Add(cmd,
		flag.Bool{
			Name:        "disable-auto-update",
			Description: "Disable automatic updates",
		},
	)

	cmd.Hidden = true

	return cmd
}

func runSaveInstall(ctx context.Context) error {
	channel := flag.GetString(ctx, "channel")
	autoUpdateEnabled := flag.GetBool(ctx, "auto-update")

	return saveInstall(ctx, channel, autoUpdateEnabled)
}

func saveInstall(ctx context.Context, channel string, autoUpdateEnabled bool) error {
	io := iostreams.FromContext(ctx)
	cache := cache.FromContext(ctx)

	cache.SetChannel(update.NormalizeChannel(channel))

	fmt.Fprintf(io.ErrOut, "set update channel to %s\n", channel)

	// TODO[md]: This was copied from internal/command/settings/autoupdate.go... move it to a helper
	// so we're not doing it twice
	path := state.ConfigFile(ctx)
	if err := config.SetAutoUpdate(path, autoUpdateEnabled); err != nil {
		return fmt.Errorf("failed persisting %s in %s: %w", config.AutoUpdateFileKey, path, err)
	}
	return nil
}
