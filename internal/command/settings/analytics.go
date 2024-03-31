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

func newAnalytics() *cobra.Command {
	metricsRoot := command.New("analytics", "Control anonymized analytics collection", "", runAnalyticsStatus)

	optIn := command.New("enable", "Enable analytics", "", func(ctx context.Context) error {
		return setAnalyticsEnabled(ctx, true)
	})
	optOut := command.New("disable", "Disable analytics", "", func(ctx context.Context) error {
		return setAnalyticsEnabled(ctx, false)
	})

	metricsRoot.AddCommand(optIn)
	metricsRoot.AddCommand(optOut)

	return metricsRoot
}

func printAnalyticsEnabled(ctx context.Context, enabled bool) {

	enabledStr := lo.Ternary(enabled, "enabled", "disabled")

	io := iostreams.FromContext(ctx)
	fmt.Fprintf(io.Out, "Anonymized analytics: %s\n", enabledStr)
}

func runAnalyticsStatus(ctx context.Context) error {
	var (
		cfg = config.FromContext(ctx)
		io  = iostreams.FromContext(ctx)
	)

	printAnalyticsEnabled(ctx, cfg.SendMetrics)

	fmt.Fprintf(io.Out, "\nThis can be controlled with 'fly settings analytics <enable/disable>'\n")

	return nil
}

func setAnalyticsEnabled(ctx context.Context, enabled bool) error {
	path := state.ConfigFile(ctx)

	if err := config.SetSendMetrics(path, enabled); err != nil {
		return fmt.Errorf("failed persisting %s in %s: %w\n",
			config.SendMetricsFileKey, path, err)
	}

	printAnalyticsEnabled(ctx, enabled)

	return nil
}
