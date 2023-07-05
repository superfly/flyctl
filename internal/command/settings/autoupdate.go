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
	autoupdateRoot := command.New("autoupdate", "Control automatic updates", "", runAutoupdateStatus)

	optIn := command.New("enable", "Enable automatic updates", "", func(ctx context.Context) error {
		return setAutoupdateEnabled(ctx, true)
	})
	optOut := command.New("disable", "Disable automatic updates", "", func(ctx context.Context) error {
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
