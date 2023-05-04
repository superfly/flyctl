package proxy

import (
	"context"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/logger"
)

func New() *cobra.Command {
	const (
		short = "Commands for proxying and interacting with Fly's proxy"
		long  = short + "\n"
		usage = "proxy <command>"
	)

	cmd := command.New(usage, short, long, runForwardWithDeprecationWarning)

	cmd.AddCommand(
		newForward(),
		newBalance(),
	)

	// TODO: remove once we deprecate `fly proxy <local:remote> [remote_host]`
	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
		flag.Org(),
		flag.Bool{
			Name:        "select",
			Shorthand:   "s",
			Default:     false,
			Description: "Prompt to select from available instances from the current application",
		},
		flag.Bool{
			Name:        "quiet",
			Shorthand:   "q",
			Description: "Don't print progress indicators for WireGuard",
		},
	)

	return cmd
}

func runForwardWithDeprecationWarning(ctx context.Context) (err error) {
	args := flag.Args(ctx)
	if len(args) == 0 {
		cmd := command.FromContext(ctx)
		return cmd.Help()
	}

	logger := logger.FromContext(ctx)
	logger.Warn("`fly proxy <local:remote> [remote_host]` is deprecated in favor of `fly proxy forward <local:remote> [remote_host]`. Usage from `fly proxy` directly will be removed in a future version.")

	ctx, err = command.RequireSession(ctx)
	if err != nil {
		return err
	}

	ctx, err = command.LoadAppNameIfPresent(ctx)
	if err != nil {
		return err
	}

	return runForward(ctx)
}
