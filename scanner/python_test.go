package scanner

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Having no dependencies isn't an error. An empty requirements.txt gets the
// generic Python setup, like one listing no framework fly launch knows, and a
// pyproject.toml without [project].dependencies (usually tool config) leaves
// the dependencies to the other files.
func TestConfigurePythonWithoutDependencies(t *testing.T) {
	t.Run("empty requirements.txt", func(t *testing.T) {
		t.Chdir(t.TempDir())
		require.NoError(t, os.WriteFile("requirements.txt", nil, 0o644))

		srcInfo, err := configurePython(".", &ScannerConfig{})

		require.NoError(t, err)
		require.NotNil(t, srcInfo)
		assert.Equal(t, "Python", srcInfo.Family)
	})

	t.Run("tool-only pyproject.toml", func(t *testing.T) {
		t.Chdir(t.TempDir())
		require.NoError(t, os.WriteFile("pyproject.toml", []byte("[tool.ruff]\nline-length = 100\n"), 0o644))
		require.NoError(t, os.WriteFile("requirements.txt", []byte("flask==3.0.0\n"), 0o644))

		srcInfo, err := configurePython(".", &ScannerConfig{})

		require.NoError(t, err)
		require.NotNil(t, srcInfo)
		assert.Equal(t, "Flask", srcInfo.Family)
	})
}
