package autoupdate

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/cache"
	"github.com/superfly/flyctl/internal/command"
)

func newSet(usage string, setting bool) (cmd *cobra.Command) {
	var (
		long  = fmt.Sprintf("Set auto-update %s", strings.ToUpper(usage))
		short = long
	)

	return command.New(usage, short, long, runSet(usage, setting))
}

func runSet(usage string, setting bool) func(ctx context.Context) (err error) {
		return func(ctx context.Context) (err error) {
				c := cache.FromContext(ctx)
				c.SetAutoUpdate(setting)
				fmt.Println("auto-update:", strings.ToUpper(usage))
				return
		}
}
