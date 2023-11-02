package metrics

import (
	"context"
	"io"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/metrics"
	"github.com/superfly/flyctl/iostreams"
)

func New() (cmd *cobra.Command) {
	const (
		short = "Commands that handle sending any metrics to flyctl-metrics"
		long  = short + "\n"
		usage = "metrics <command>"
	)

	cmd = command.New(usage, short, long, nil)
	cmd.Hidden = true

	cmd.AddCommand(
		newSend(),
	)

	return
}

func newSend() (cmd *cobra.Command) {
	const (
		short = "Send any metrics in stdin to flyctl-metrics"
		long  = short + "\n"
	)

	cmd = command.New("send", short, long, run, func(ctx context.Context) (context.Context, error) {
		return metrics.WithDisableFlushMetrics(ctx), nil
	})
	cmd.Hidden = true
	cmd.Args = cobra.NoArgs

	return
}

func run(ctx context.Context) error {
	iostream := iostreams.FromContext(ctx)
	stdin := iostream.In

	stdin_bytes, err := io.ReadAll(stdin)
	if err != nil {
		return err
	}

	stdin_value := string(stdin_bytes)

	return metrics.SendMetrics(ctx, stdin_value)
}
