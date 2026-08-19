package launch

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/require"
	"github.com/superfly/flyctl/internal/appconfig"
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
