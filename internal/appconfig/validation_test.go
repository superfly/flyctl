package appconfig

import (
	"context"
	"os"
	"testing"

	getsentry "github.com/getsentry/sentry-go"
	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/internal/cmdutil/preparers"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/logger"
)

func TestConfigValidateRedactsMalformedDockerfileURL(t *testing.T) {
	dockerfileURL := "https://" + "user:pass@" + "example.com/%zz?token=secret#fragment"
	cfg := &Config{Build: &Build{
		Builder:    "example.com/builder",
		Dockerfile: dockerfileURL,
	}}

	var captured *getsentry.Event
	client, err := getsentry.NewClient(getsentry.ClientOptions{
		BeforeSend: func(event *getsentry.Event, _ *getsentry.EventHint) *getsentry.Event {
			captured = event

			return nil
		},
	})
	require.NoError(t, err)
	hub := getsentry.CurrentHub()
	previousClient := hub.Client()
	hub.BindClient(client)
	t.Cleanup(func() { hub.BindClient(previousClient) })

	err, output := cfg.Validate(context.Background())

	require.NoError(t, err)
	assert.Contains(t, output, `the "invalid URL" dockerfile`)
	assert.NotContains(t, output, "user:pass@")
	assert.NotContains(t, output, "token=secret")
	assert.NotContains(t, output, "#fragment")
	require.NotNil(t, captured)
	require.NotEmpty(t, captured.Exception)
	assert.Contains(t, captured.Exception[0].Value, "invalid URL")
	assert.NotContains(t, captured.Exception[0].Value, "user:pass@")
	assert.NotContains(t, captured.Exception[0].Value, "token=secret")
	assert.NotContains(t, captured.Exception[0].Value, "#fragment")
}

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

func TestValidateMachineConversionReportsGuestError(t *testing.T) {
	cfg := &Config{
		Compute: []*Compute{{Memory: "not-a-size"}},
	}

	extra, err := cfg.validateMachineConversion()

	require.ErrorIs(t, err, ErrInvalidApplicationConfig)
	require.Contains(t, extra, "process group 'app'")
	require.Contains(t, extra, "not-a-size")
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

func TestConfig_ValidateServicesOneProcessGroup(t *testing.T) {
	cfg, err := LoadConfig("./testdata/validate-services-one-group.toml")
	require.NoError(t, err)
	require.NoError(t, cfg.SetMachinesPlatform())

	err, x := cfg.Validate(_getValidationContext(t))
	require.NoErrorf(t, err, x)

	mConfig, err := cfg.ToMachineConfig("web", nil)
	require.NoError(t, err)
	require.Len(t, mConfig.Services, 2)
	assert.Equal(t, 8080, mConfig.Services[0].InternalPort)
	assert.Equal(t, 9000, mConfig.Services[1].InternalPort)
}

func TestConfig_ValidateServicesTwoProcessGroups(t *testing.T) {
	cfg, err := LoadConfig("./testdata/validate-services-two-groups.toml")
	require.NoError(t, err)
	require.NoError(t, cfg.SetMachinesPlatform())

	err, x := cfg.Validate(_getValidationContext(t))
	require.Error(t, err, x)
	require.Contains(t, x, "Service has no processes set but app has 2 processes defined")
}

func TestCacheDiskValidation(t *testing.T) {
	for _, tc := range []struct {
		name  string
		disks []CacheDisk
		want  string
	}{
		{"missing size", []CacheDisk{{Path: "/cache"}}, "cache_disk.size"},
		{"zero size", []CacheDisk{{Size: "0mib", Path: "/cache"}}, "cache_disk.size must be greater than zero"},
		{"negative size", []CacheDisk{{Size: "-1mib", Path: "/cache"}}, "invalid cache_disk.size"},
		{"unitless size", []CacheDisk{{Size: "1024", Path: "/cache"}}, "requires an explicit unit"},
		{"fractional unitless size", []CacheDisk{{Size: "1.5", Path: "/cache"}}, "requires an explicit unit"},
		{"invalid size", []CacheDisk{{Size: "invalid", Path: "/cache"}}, "invalid cache_disk.size"},
		{"missing path", []CacheDisk{{Size: "1024mib"}}, "path is required"},
		{"overlap", []CacheDisk{{Size: "1024mib", Path: "/a"}, {Size: "1024mib", Path: "/b", Processes: []string{"app"}}}, `process group "app" has more than one [[cache_disk]] entry; only one cache disk is allowed per Machine`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := NewConfig()
			cfg.CacheDisks = tc.disks
			_, err := cfg.ToMachineConfig("app", nil)
			require.ErrorContains(t, err, tc.want)
		})
	}
	cfg := NewConfig()
	cfg.CacheDisks = []CacheDisk{{Size: "1024mib", Path: "/cache", Processes: []string{"missing"}}}
	info, err := cfg.validateCacheDisks()
	require.Error(t, err)
	require.Contains(t, info, "unknown process group")
	// Limits and path rules belong to the server, including future changes.
	cfg.CacheDisks = []CacheDisk{{Size: "1mib", Path: "/cache"}}
	_, err = cfg.ToMachineConfig("app", nil)
	require.NoError(t, err)
	cfg.CacheDisks[0].Size = "100000mib"
	_, err = cfg.ToMachineConfig("app", nil)
	require.NoError(t, err)
}

func TestValidateComputeMemoryAndMemoryMBWarning(t *testing.T) {
	t.Run("warns when both memory and memory_mb are set in [[vm]]", func(t *testing.T) {
		cfg := NewConfig()
		cfg.Compute = []*Compute{
			{
				Memory: "2gb",
				MachineGuest: &fly.MachineGuest{
					MemoryMB: 1024,
				},
			},
		}

		err, extraInfo := cfg.Validate(context.Background())
		require.NoError(t, err)
		assert.Contains(t, extraInfo, "WARN")
		assert.Contains(t, extraInfo, "memory takes precedence over memory_mb")
	})

	t.Run("warns when both memory and memory_mb are set in release_command_vm", func(t *testing.T) {
		cfg := NewConfig()
		cfg.Deploy = &Deploy{
			ReleaseCommandCompute: &Compute{
				Memory: "1gb",
				MachineGuest: &fly.MachineGuest{
					MemoryMB: 512,
				},
			},
		}

		err, extraInfo := cfg.Validate(context.Background())
		require.NoError(t, err)
		assert.Contains(t, extraInfo, "WARN")
		assert.Contains(t, extraInfo, "[deploy.release_command_vm]")
		assert.Contains(t, extraInfo, "memory takes precedence over memory_mb")
	})

	t.Run("no warning when only memory is set", func(t *testing.T) {
		cfg := NewConfig()
		cfg.Compute = []*Compute{
			{
				Memory: "2gb",
			},
		}

		err, extraInfo := cfg.Validate(context.Background())
		require.NoError(t, err)
		assert.NotContains(t, extraInfo, "memory takes precedence over memory_mb")
	})
}
