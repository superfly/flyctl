package appconfig

import (
	"context"
	"os"
	"testing"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/require"
	"github.com/superfly/flyctl/internal/cmdutil/preparers"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/logger"
)

func _getValidationContext(t *testing.T) context.Context {
	ctx := logger.NewContext(context.Background(), logger.New(os.Stderr, logger.Info, false))
	ctx = flag.NewContext(ctx, &pflag.FlagSet{})
	ctx, err := preparers.DetermineConfigDir(ctx)
	require.NoError(t, err)
	ctx, err = preparers.LoadConfig(ctx)
	require.NoError(t, err)
	ctx, err = preparers.InitClient(ctx)
	require.NoError(t, err)
	return ctx
}

func TestConfig_ValidateGroups(t *testing.T) {
	cfg, err := LoadConfig("./testdata/validate-groups.toml")
	require.NoError(t, err)
	require.NoError(t, cfg.SetMachinesPlatform())
	cfg.Deploy = &Deploy{Strategy: "canary"}

	ctx := _getValidationContext(t)
	err, x := cfg.Validate(ctx)
	require.Error(t, err, x)
	require.Contains(t, x, "error canary deployment strategy is not supported when using mounted volumes")

	err, x = cfg.ValidateGroups(ctx, []string{"app"})
	require.Error(t, err, x)

	err, x = cfg.ValidateGroups(ctx, []string{"foo"})
	require.NoErrorf(t, err, x)
}

func TestConfig_ValidateMounts(t *testing.T) {
	cfg, err := LoadConfig("./testdata/validate-mounts.toml")
	require.NoError(t, err)
	require.NoError(t, cfg.SetMachinesPlatform())

	ctx := _getValidationContext(t)
	err, x := cfg.Validate(ctx)
	require.Error(t, err, x)
	require.Contains(t, x, "has an initial_size '15Mb' value which is smaller than 1GB")

	err, x = cfg.ValidateGroups(ctx, []string{"app"})
	require.Error(t, err, x)
	require.Contains(t, x, "group 'app' has more than one [[mounts]] section defined")
}

func TestConfig_ValidateServices(t *testing.T) {
	cfg, err := LoadConfig("./testdata/validate-services.toml")
	require.NoError(t, err)
	require.NoError(t, cfg.SetMachinesPlatform())

	ctx := _getValidationContext(t)
	err, x := cfg.Validate(ctx)
	require.Error(t, err, x)
	require.Contains(t, x, "Service has no processes set")
	require.Contains(t, x, "Service must expose at least one port")

	err, x = cfg.ValidateGroups(ctx, []string{"success"})
	require.NoErrorf(t, err, x)
}
