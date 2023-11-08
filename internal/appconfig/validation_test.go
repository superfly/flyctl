package appconfig

import (
	"context"
	"github.com/spf13/pflag"
	"github.com/stretchr/testify/require"
	"github.com/superfly/flyctl/internal/cmdutil/preparers"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/logger"
	"os"
	"testing"
)

func TestConfig_ValidateGroups(t *testing.T) {
	ctx := logger.NewContext(context.Background(), logger.New(os.Stderr, logger.Info, false))
	ctx = flag.NewContext(ctx, &pflag.FlagSet{})
	ctx, err := preparers.DetermineUserHomeDir(ctx)
	require.NoError(t, err)
	ctx, err = preparers.DetermineConfigDir(ctx)
	require.NoError(t, err)
	ctx, err = preparers.LoadConfig(ctx)
	require.NoError(t, err)
	ctx, err = preparers.InitClient(ctx)
	require.NoError(t, err)

	cfg, err := LoadConfig("./testdata/validategroups.toml")
	require.NoError(t, err)
	require.NoError(t, cfg.SetMachinesPlatform())
	cfg.Deploy = &Deploy{Strategy: "canary"}

	err, x := cfg.Validate(ctx)
	require.Error(t, err, x)
	require.Contains(t, x, "error canary deployment strategy is not supported when using mounted volumes")

	err, x = cfg.ValidateGroups(ctx, []string{"app"})
	require.Error(t, err, x)

	err, _ = cfg.ValidateGroups(ctx, []string{"foo"})
	require.NoError(t, err)
}
