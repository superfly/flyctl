package apps

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/iostreams"
)

func TestRunOpenFallsBackToDefaultAppURLWhenRemoteConfigIsUnavailable(t *testing.T) {
	ctx, out := testOpenContext(t)
	ctx = appconfig.WithName(ctx, "test-app")

	restore := stubOpenDependencies(t)
	defer restore()

	loadRemoteAppConfig = func(context.Context, string) (*appconfig.Config, error) {
		return nil, errors.New("missing remote config")
	}

	require.NoError(t, runOpen(ctx))
	assert.Equal(t, "https://test-app.fly.dev/", openedURL)
	assert.Contains(t, out.String(), "opening https://test-app.fly.dev/ ...")
}

func TestRunOpenUsesLocalConfigURLWhenPresent(t *testing.T) {
	ctx, out := testOpenContext(t, "dashboard")
	ctx = appconfig.WithName(ctx, "test-app")
	ctx = appconfig.WithConfig(ctx, &appconfig.Config{
		AppName:     "test-app",
		HTTPService: &appconfig.HTTPService{InternalPort: 8080},
	})

	restore := stubOpenDependencies(t)
	defer restore()

	loadRemoteAppConfig = func(context.Context, string) (*appconfig.Config, error) {
		t.Fatal("should not load remote app config when local config exists")

		return nil, nil
	}

	require.NoError(t, runOpen(ctx))
	assert.Equal(t, "https://test-app.fly.dev/dashboard", openedURL)
	assert.Contains(t, out.String(), "opening https://test-app.fly.dev/dashboard ...")
}

func TestRunOpenFallsBackToSelectedAppWhenLocalConfigIsForAnotherApp(t *testing.T) {
	ctx, out := testOpenContext(t)
	ctx = appconfig.WithName(ctx, "flag-app")
	ctx = appconfig.WithConfig(ctx, &appconfig.Config{
		AppName:     "local-app",
		HTTPService: &appconfig.HTTPService{InternalPort: 8080},
	})

	restore := stubOpenDependencies(t)
	defer restore()

	var remoteAppName string
	loadRemoteAppConfig = func(_ context.Context, appName string) (*appconfig.Config, error) {
		remoteAppName = appName

		return nil, errors.New("missing remote config")
	}

	require.NoError(t, runOpen(ctx))
	assert.Equal(t, "flag-app", remoteAppName)
	assert.Equal(t, "https://flag-app.fly.dev/", openedURL)
	assert.Contains(t, out.String(), "opening https://flag-app.fly.dev/ ...")
}

func TestRunOpenUsesRemoteConfigURLWhenAvailable(t *testing.T) {
	ctx, out := testOpenContext(t, "status")
	ctx = appconfig.WithName(ctx, "test-app")

	restore := stubOpenDependencies(t)
	defer restore()

	loadRemoteAppConfig = func(context.Context, string) (*appconfig.Config, error) {
		return &appconfig.Config{
			AppName:     "test-app",
			HTTPService: &appconfig.HTTPService{InternalPort: 8080},
		}, nil
	}

	require.NoError(t, runOpen(ctx))
	assert.Equal(t, "https://test-app.fly.dev/status", openedURL)
	assert.Contains(t, out.String(), "opening https://test-app.fly.dev/status ...")
}

func TestRunOpenDoesNotFallbackWhenRemoteConfigHasNoPublicService(t *testing.T) {
	ctx, _ := testOpenContext(t)
	ctx = appconfig.WithName(ctx, "test-app")

	restore := stubOpenDependencies(t)
	defer restore()

	loadRemoteAppConfig = func(context.Context, string) (*appconfig.Config, error) {
		return &appconfig.Config{AppName: "test-app"}, nil
	}

	err := runOpen(ctx)
	require.EqualError(t, err, "The app doesn't expose a public http service")
	assert.Empty(t, openedURL)
}

var openedURL string

func stubOpenDependencies(t *testing.T) func() {
	t.Helper()

	openedURL = ""
	originalOpenBrowser := openBrowser
	originalLoadRemoteAppConfig := loadRemoteAppConfig

	openBrowser = func(url string) error {
		openedURL = url

		return nil
	}

	return func() {
		openBrowser = originalOpenBrowser
		loadRemoteAppConfig = originalLoadRemoteAppConfig
	}
}

func testOpenContext(t *testing.T, args ...string) (context.Context, *bytes.Buffer) {
	t.Helper()

	flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
	require.NoError(t, flags.Parse(args))

	out := &bytes.Buffer{}
	ctx := flag.NewContext(context.Background(), flags)
	ctx = iostreams.NewContext(ctx, &iostreams.IOStreams{
		Out:    out,
		ErrOut: out,
	})

	return ctx, out
}
