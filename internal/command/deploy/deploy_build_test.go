package deploy

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/launchdarkly"
	"github.com/superfly/flyctl/internal/state"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestMultipleDockerfile(t *testing.T) {
	dir := t.TempDir()

	dockerfile, err := os.Create(filepath.Join(dir, "Dockerfile"))
	require.NoError(t, err)
	defer dockerfile.Close() // skipcq: GO-S2307

	flyToml, err := os.Create(filepath.Join(dir, "fly.production.toml"))
	require.NoError(t, err)
	defer flyToml.Close() // skipcq: GO-S2307

	cfg, err := appconfig.LoadConfig(flyToml.Name())
	require.NoError(t, err)
	cfg.Build = &appconfig.Build{
		Dockerfile: "Dockerfile.from-fly-toml",
	}

	ctx := state.WithWorkingDirectory(context.Background(), dir)
	err = multipleDockerfile(ctx, &appconfig.Config{})

	assert.NoError(t, err)

	err = multipleDockerfile(ctx, cfg)
	assert.ErrorContains(t, err, "fly.production.toml")

	t.Run("redacts credentials in URL warning", func(t *testing.T) {
		cfg.Build.Dockerfile = "https://" + "user:password@" + "example.com/Dockerfile?token=secret#fragment"

		err := multipleDockerfile(ctx, cfg)

		require.Error(t, err)
		assert.ErrorContains(t, err, "https://example.com/Dockerfile")
		assert.NotContains(t, err.Error(), "user")
		assert.NotContains(t, err.Error(), "password")
		assert.NotContains(t, err.Error(), "token")
		assert.NotContains(t, err.Error(), "secret")
		assert.NotContains(t, err.Error(), "fragment")
	})
}

func TestResolveDockerfilePath(t *testing.T) {
	t.Run("relative config path", func(t *testing.T) {
		dir := t.TempDir()
		cfg := &appconfig.Config{
			Build: &appconfig.Build{Dockerfile: "Dockerfile.custom"},
		}
		cfg.SetConfigFilePath(filepath.Join(dir, "fly.toml"))

		got, err := resolveDockerfilePath(context.Background(), cfg)

		require.NoError(t, err)
		assert.Equal(t, filepath.Join(dir, "Dockerfile.custom"), got)
	})

	t.Run("URL remains unchanged", func(t *testing.T) {
		const dockerfileURL = "https://example.com/Dockerfile?token=secret"
		cfg := &appconfig.Config{
			Build: &appconfig.Build{Dockerfile: dockerfileURL},
		}

		got, err := resolveDockerfilePath(context.Background(), cfg)

		require.NoError(t, err)
		assert.Equal(t, dockerfileURL, got)
	})

	for name, dockerfileURL := range map[string]string{
		"invalid escape": "https://" + "user:pass@" + "example.com/%zz?token=secret#fragment",
		"missing slash":  "https:/" + "user:pass@" + "example.com/Dockerfile?token=secret",
		"backslash form": `https:\user:pass@example.com\Dockerfile?token=secret`,
	} {
		t.Run("malformed URL fails closed: "+name, func(t *testing.T) {
			cfg := &appconfig.Config{
				Build: &appconfig.Build{Dockerfile: dockerfileURL},
			}

			got, err := resolveDockerfilePath(context.Background(), cfg)

			assert.Empty(t, got)
			require.Error(t, err)
			assert.EqualError(t, err, "invalid Dockerfile URL")
			assert.NotContains(t, err.Error(), dockerfileURL)
		})
	}
}

// Ensure invalid source options fail inside a dedicated validation span beneath image preparation.
func TestValidateImageInputsSpan(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	previous := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	t.Cleanup(func() {
		otel.SetTracerProvider(previous)
		_ = provider.Shutdown(context.Background())
	})
	flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
	flags.StringArray("build-arg", []string{"invalid"}, "")
	ctx := flag.NewContext(context.Background(), flags)
	ctx = state.WithWorkingDirectory(ctx, t.TempDir())
	ctx = launchdarkly.NewContextWithClient(ctx, &launchdarkly.Client{})
	ctx, parent := provider.Tracer("test").Start(ctx, "determine_image")
	_, err := validateImageInputs(ctx, &appconfig.Config{}, &appconfig.Build{})
	parent.End()
	require.ErrorContains(t, err, "invalid build args")
	var validation sdktrace.ReadOnlySpan
	for _, span := range recorder.Ended() {
		if span.Name() == "validate_image_inputs" {
			validation = span
		}
	}
	require.NotNil(t, validation)
	assert.Equal(t, parent.SpanContext().SpanID(), validation.Parent().SpanID())
	assert.Equal(t, codes.Error, validation.Status().Code)
	assert.NotEmpty(t, validation.Events())
}
