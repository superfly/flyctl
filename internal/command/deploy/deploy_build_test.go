package deploy

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/state"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMultipleDockerfile(t *testing.T) {
	dir := t.TempDir()

	f, err := os.Create(filepath.Join(dir, "Dockerfile"))
	require.NoError(t, err)
	defer f.Close() // skipcq: GO-S2307

	ctx := state.WithWorkingDirectory(context.Background(), dir)
	err = multipleDockerfile(ctx, &appconfig.Config{})
	assert.NoError(t, err)

	err = multipleDockerfile(
		ctx,
		&appconfig.Config{
			Build: &appconfig.Build{
				Dockerfile: "Dockerfile.from-fly-toml",
			},
		},
	)
	assert.Error(t, err)
}
