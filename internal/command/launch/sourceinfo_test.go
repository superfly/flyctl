package launch

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command/launch/plan"
	"github.com/superfly/flyctl/internal/flag/flagctx"
	"github.com/superfly/flyctl/iostreams"
)

func TestDetermineSourceInfoDockerfileDetection(t *testing.T) {
	t.Setenv("OPT_OUT_GITHUB_ACTIONS", "1")

	tests := []struct {
		name               string
		files              map[string]string
		expectedDockerfile string
		expectedBuildPath  string
	}{
		{
			name: "Containerfile",
			files: map[string]string{
				"Containerfile": "FROM alpine\nEXPOSE 3000\n",
			},
			expectedDockerfile: "Containerfile",
			expectedBuildPath:  "Containerfile",
		},
		{
			name: "Dockerfile takes precedence",
			files: map[string]string{
				"Containerfile": "FROM alpine\nEXPOSE 3000\n",
				"Dockerfile":    "FROM alpine\nEXPOSE 8080\n",
			},
			expectedDockerfile: "Dockerfile",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			for name, contents := range tt.files {
				require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(contents), 0o644))
			}

			io, _, _, _ := iostreams.Test()
			ctx := iostreams.NewContext(context.Background(), io)
			ctx = flagctx.NewContext(ctx, pflag.NewFlagSet("test", pflag.ContinueOnError))

			sourceInfo, build, err := determineSourceInfo(ctx, appconfig.NewConfig(), false, dir)
			require.NoError(t, err)
			require.NotNil(t, sourceInfo)
			require.NotNil(t, build)
			require.Equal(t, filepath.Join(dir, tt.expectedDockerfile), sourceInfo.DockerfilePath)
			require.Equal(t, tt.expectedBuildPath, build.Dockerfile)
		})
	}
}

// newSourceInfoCtx carries every launch flag, with or without a terminal.
func newSourceInfoCtx(t *testing.T, terminal bool, args ...string) context.Context {
	t.Helper()

	ios, _, _, _ := iostreams.Test()
	ios.SetStdinTTY(terminal)
	ios.SetStdoutTTY(terminal)
	ctx := iostreams.NewContext(context.Background(), ios)

	flags := New().Flags()
	require.NoError(t, flags.Parse(args))

	return flagctx.NewContext(ctx, flags)
}

// A blank app is never deployed. In a terminal someone reads "Continuing with
// a blank app"; headless, the empty app and exit 0 pass for a successful
// launch, so fail before anything is created unless --no-deploy asks for it.
func TestDetermineSourceInfoNothingToBuild(t *testing.T) {
	t.Run("headless fails with directions", func(t *testing.T) {
		_, _, err := determineSourceInfo(newSourceInfoCtx(t, false), appconfig.NewConfig(), false, t.TempDir())

		require.Error(t, err)
		assert.Contains(t, err.Error(), "Dockerfile")
		assert.Contains(t, err.Error(), "--no-deploy")
	})

	t.Run("headless --no-deploy continues with a blank app", func(t *testing.T) {
		srcInfo, _, err := determineSourceInfo(newSourceInfoCtx(t, false, "--no-deploy"), appconfig.NewConfig(), false, t.TempDir())

		require.NoError(t, err)
		assert.Nil(t, srcInfo)
	})

	t.Run("headless --manifest only prints the plan", func(t *testing.T) {
		// `fly launch --manifest > plan.json` creates and deploys nothing, so
		// there's no false success to prevent.
		srcInfo, _, err := determineSourceInfo(newSourceInfoCtx(t, false, "--manifest"), appconfig.NewConfig(), false, t.TempDir())

		require.NoError(t, err)
		assert.Nil(t, srcInfo)
	})

	t.Run("headless --no-create-app only writes fly.toml", func(t *testing.T) {
		srcInfo, _, err := determineSourceInfo(newSourceInfoCtx(t, false, "--no-create-app"), appconfig.NewConfig(), false, t.TempDir())

		require.NoError(t, err)
		assert.Nil(t, srcInfo)
	})

	t.Run("terminal continues with a blank app", func(t *testing.T) {
		srcInfo, _, err := determineSourceInfo(newSourceInfoCtx(t, true), appconfig.NewConfig(), false, t.TempDir())

		require.NoError(t, err)
		assert.Nil(t, srcInfo)
	})

	t.Run("plan propose keeps its own error", func(t *testing.T) {
		// The deployer runs propose headless; it has no --no-deploy to suggest.
		ctx := context.WithValue(newSourceInfoCtx(t, false), plan.PlanStepKey, "propose")

		_, _, err := determineSourceInfo(ctx, appconfig.NewConfig(), false, t.TempDir())

		require.Error(t, err)
		assert.NotContains(t, err.Error(), "--no-deploy")
	})
}
