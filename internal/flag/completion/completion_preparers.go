package completion

import (
	"context"
	"time"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/cmdutil/preparers"
	"github.com/superfly/flyctl/internal/flag/flagctx"
	"github.com/superfly/flyctl/internal/logger"
	"github.com/superfly/flyctl/iostreams"
	"github.com/superfly/flyctl/terminal"
)

func prepareInitialCtx(cmd *cobra.Command) (context.Context, error) {

	io := iostreams.System()
	ctx := cmd.Context()
	ctx = flagctx.NewContext(ctx, cmd.Flags())
	ctx, err := preparers.ApplyAliases(ctx)
	if err != nil {
		return nil, err
	}
	ctx, err = preparers.DetermineConfigDir(ctx)
	if err != nil {
		return nil, err
	}
	ctx = iostreams.NewContext(ctx, io)
	ctx = logger.NewContext(ctx, logger.FromEnv(io.ErrOut))
	ctx, err = preparers.LoadConfig(ctx)
	if err != nil {
		return nil, err
	}
	return ctx, nil
}

// Adapt takes a function that expects a standard flyctl command context and returns completion ideas or an error,
// and adapts it to the completion function signature expected by cobra.
// It also sets up the context with a subset of the preparers that are needed for basic functions of flyctl.
func Adapt(
	fn func(ctx context.Context, cmd *cobra.Command, args []string, partial string) ([]string, error),
) func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
	return func(cmd *cobra.Command, args []string, partial string) (ideas []string, code cobra.ShellCompDirective) {

		var err error
		defer func() {
			if code == cobra.ShellCompDirectiveError {
				terminal.Debugf("completion error: %v\n", err)
			}
		}()

		ctx, err := prepareInitialCtx(cmd)
		if err != nil {
			return nil, cobra.ShellCompDirectiveError
		}

		ctx, cancelFn := context.WithTimeout(ctx, 5*time.Second)
		defer cancelFn()

		ctx, err = preparers.InitClient(ctx)
		if err != nil {
			return nil, cobra.ShellCompDirectiveError
		}

		res, err := fn(ctx, cmd, args, partial)
		if err != nil {
			return nil, cobra.ShellCompDirectiveError
		} else {
			return res, cobra.ShellCompDirectiveNoFileComp
		}
	}
}
