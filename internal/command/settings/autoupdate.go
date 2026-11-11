package settings

import (
	"context"
	"fmt"

	"github.com/samber/lo"
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/state"
	"github.com/superfly/flyctl/iostreams"
)

func newAutoUpdate() *cobra.Command {
	const (
		long = `Show whether fly updates itself automatically. Automatic updates are on
by default. Change the setting with ` + "`fly settings autoupdate enable`" + ` or
` + "`fly settings autoupdate disable`" + `.`
		enableLong = `Turn on automatic updates for fly, and save the setting in your fly
config file. When a newer release is available, fly updates itself in the
background (except on Windows), or before running your command if the
installed version is far behind. Updates never run in CI or for
installations fly can't update itself.`
		disableLong = `Turn off automatic updates for fly, and save the setting in your fly
config file. Update manually with ` + "`fly version upgrade`" + `.`
	)

	autoupdateRoot := command.New("autoupdate", "Control automatic updates", long, runAutoupdateStatus)

	optIn := command.New("enable", "Enable automatic updates", enableLong, func(ctx context.Context) error {
		return setAutoupdateEnabled(ctx, true)
	})
	optOut := command.New("disable", "Disable automatic updates", disableLong, func(ctx context.Context) error {
		return setAutoupdateEnabled(ctx, false)
	})

	autoupdateRoot.AddCommand(optIn)
	autoupdateRoot.AddCommand(optOut)

	return autoupdateRoot
}

func printAutoupdateEnabled(ctx context.Context, enabled bool) {

	enabledStr := lo.Ternary(enabled, "enabled", "disabled")

	io := iostreams.FromContext(ctx)
	fmt.Fprintf(io.Out, "Automatic updating: %s\n", enabledStr)
}

func runAutoupdateStatus(ctx context.Context) error {
	var (
		cfg = config.FromContext(ctx)
		io  = iostreams.FromContext(ctx)
	)

	printAutoupdateEnabled(ctx, cfg.AutoUpdate)

	fmt.Fprintf(io.Out, "\nThis can be controlled with 'fly settings autoupdate <enable/disable>'\n")

	return nil
}

func setAutoupdateEnabled(ctx context.Context, enabled bool) error {
	path := state.ConfigFile(ctx)

	if err := config.SetAutoUpdate(path, enabled); err != nil {
		return fmt.Errorf("failed persisting %s in %s: %w\n",
			config.AutoUpdateFileKey, path, err)
	}

	printAutoupdateEnabled(ctx, enabled)

	return nil
}
