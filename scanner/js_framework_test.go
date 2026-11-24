package scanner

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDetectJSPackageManager(t *testing.T) {
	t.Run("defaults to npm", func(t *testing.T) {
		dir := t.TempDir()
		require.Equal(t, "npm", detectJSPackageManager(dir, nil))
	})

	t.Run("detects from lockfiles", func(t *testing.T) {
		cases := map[string]string{
			"package-lock.json": "npm",
			"yarn.lock":         "yarn",
			"pnpm-lock.yaml":    "pnpm",
			"bun.lockb":         "bun",
			"bun.lock":          "bun",
		}
		for lockfile, expected := range cases {
			dir := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(dir, lockfile), []byte(""), 0o644))
			require.Equal(t, expected, detectJSPackageManager(dir, nil))
		}
	})

	t.Run("detects from parent monorepo workspace", func(t *testing.T) {
		root := t.TempDir()
		require.NoError(t, os.Mkdir(filepath.Join(root, ".git"), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(root, "pnpm-lock.yaml"), []byte(""), 0o644))
		appDir := filepath.Join(root, "apps", "web")
		require.NoError(t, os.MkdirAll(appDir, 0o755))

		require.Equal(t, "pnpm", detectJSPackageManager(appDir, nil))
	})

	t.Run("detects from packageManager field in package.json", func(t *testing.T) {
		dir := t.TempDir()
		pkg := map[string]any{"packageManager": "pnpm@9.15.0"}
		require.Equal(t, "pnpm", detectJSPackageManager(dir, pkg))
	})
}

func TestBuildDockerfileInstallArgs(t *testing.T) {
	t.Run("turborepo monorepo pnpm app uses pnpm add without -w", func(t *testing.T) {
		root := t.TempDir()
		require.NoError(t, os.Mkdir(filepath.Join(root, ".git"), 0o755))
		require.NoError(t, os.Mkdir(filepath.Join(root, "node_modules"), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(root, "pnpm-lock.yaml"), []byte(""), 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(root, "pnpm-workspace.yaml"), []byte("packages:\n  - apps/*\n"), 0o644))
		appDir := filepath.Join(root, "apps", "web")
		require.NoError(t, os.MkdirAll(appDir, 0o755))

		args := buildDockerfileInstallArgs(appDir, nil, nil)
		require.Equal(t, []string{"pnpm", "add", "-D", "@flydotio/dockerfile@latest"}, args)
	})

	t.Run("pnpm workspace root passes -w", func(t *testing.T) {
		root := t.TempDir()
		require.NoError(t, os.Mkdir(filepath.Join(root, "node_modules"), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(root, "pnpm-lock.yaml"), []byte(""), 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(root, "pnpm-workspace.yaml"), []byte(""), 0o644))

		args := buildDockerfileInstallArgs(root, nil, nil)
		require.Equal(t, []string{"pnpm", "add", "-w", "-D", "@flydotio/dockerfile@latest"}, args)
	})

	t.Run("yarn project uses yarn add", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, os.Mkdir(filepath.Join(dir, "node_modules"), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "yarn.lock"), []byte(""), 0o644))

		args := buildDockerfileInstallArgs(dir, nil, nil)
		require.Equal(t, []string{"yarn", "add", "@flydotio/dockerfile@latest", "--dev"}, args)
	})

	t.Run("bun project without node_modules uses bunx", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "bun.lock"), []byte(""), 0o644))

		args := buildDockerfileInstallArgs(dir, nil, []string{"--skip"})
		require.Equal(t, []string{"bunx", "@flydotio/dockerfile@latest", "--skip"}, args)
	})
}
