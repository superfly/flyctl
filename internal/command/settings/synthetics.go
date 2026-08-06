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

func newSynthetics() *cobra.Command {
	metricsRoot := command.New("synthetics", "Control synthetics agent execution", "", runSyntheticsStatus)

	optIn := command.New("enable", "Enable synthetics", "", func(ctx context.Context) error {
		return setSyntheticsCfg(ctx, true)
	})
	optOut := command.New("disable", "Disable synthetics", "", func(ctx context.Context) error {
		return setSyntheticsCfg(ctx, false)
	})

	metricsRoot.AddCommand(optIn)
	metricsRoot.AddCommand(optOut)

	return metricsRoot
}

func printSyntheticsEnabled(ctx context.Context, enabled bool) {

	enabledStr := lo.Ternary(enabled, "enabled", "disabled")

	io := iostreams.FromContext(ctx)
	fmt.Fprintf(io.Out, "Synthetics: %s\n", enabledStr)
}

func runSyntheticsStatus(ctx context.Context) error {
	var (
		cfg = config.FromContext(ctx)
		io  = iostreams.FromContext(ctx)
	)

	printSyntheticsEnabled(ctx, cfg.SyntheticsAgent)

	fmt.Fprintf(io.Out, "\nThis can be controlled with 'fly settings synthetics <enable/disable>'\n")

	return nil
}

func setSyntheticsCfg(ctx context.Context, enabled bool) error {
	path := state.ConfigFile(ctx)

	if err := config.SetSyntheticsAgent(path, enabled); err != nil {
		return fmt.Errorf("failed persisting %s in %s: %w\n",
			config.SyntheticsAgentFileKey, path, err)
	}

	printSyntheticsEnabled(ctx, enabled)

	return nil
}
