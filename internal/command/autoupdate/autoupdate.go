package autoupdate

import (
	"context"
	"fmt"

	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/cache"

	"github.com/spf13/cobra"
)

func New() *cobra.Command {
	const (
		short = "Auto-update setting for flyctl CLI"

		long = `Auto-update setting for flyctl command. Default is OFF.`
	)

	cmd := command.New("autoupdate", short, long, runAutoUpdate)

	cmd.AddCommand(
		newSet("on", true),
		newSet("off", false),
	)

	return cmd
}

func runAutoUpdate(ctx context.Context) (err error) {
	cache := cache.FromContext(ctx)

	var autoUpdate string;
	if (cache.AutoUpdate()) {
		autoUpdate = "ON"
	} else {
		autoUpdate = "OFF"
	}

	fmt.Println("auto-update:", autoUpdate)
	return
}

