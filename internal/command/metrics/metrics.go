package metrics

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

func New() *cobra.Command {
	metricsRoot := command.New("metrics", "Control client metrics collection", "", runStatus)

	optIn := command.New("opt-in", "Opt-in to metrics collection", "", func(ctx context.Context) error {
		return setMetricsEnabled(ctx, true)
	})
	optOut := command.New("opt-out", "Out-out of metrics collection", "", func(ctx context.Context) error {
		return setMetricsEnabled(ctx, false)
	})

	metricsRoot.AddCommand(optIn)
	metricsRoot.AddCommand(optOut)

	return metricsRoot
}

func printEnabled(ctx context.Context, enabled bool) {

	enabledStr := lo.Ternary(enabled, "enabled", "disabled")

	io := iostreams.FromContext(ctx)
	fmt.Fprintf(io.Out, "Anonymized metrics: %s\n", enabledStr)
}

func runStatus(ctx context.Context) error {
	var (
		cfg = config.FromContext(ctx)
		io  = iostreams.FromContext(ctx)
	)

	printEnabled(ctx, cfg.SendMetrics)

	fmt.Fprintf(io.Out, "\nThis can be controlled with flyctl metrics <opt-in/opt-out>\n")

	return nil
}

func setMetricsEnabled(ctx context.Context, enabled bool) error {
	path := state.ConfigFile(ctx)

	if err := config.SetSendMetrics(path, enabled); err != nil {
		return fmt.Errorf("failed persisting %s in %s: %w\n",
			config.SendMetricsFileKey, path, err)
	}

	printEnabled(ctx, enabled)

	return nil
}
