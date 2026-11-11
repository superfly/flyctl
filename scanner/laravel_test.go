package scanner

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeLaravelApp(t *testing.T, phpRequirement string) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "artisan"), []byte("#!/usr/bin/env php\n"), 0o755))
	composer := `{"require": {"php": "` + phpRequirement + `"}}`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "composer.json"), []byte(composer), 0o644))

	return dir
}

func TestConfigureLaravelRejectsPhpBelow81(t *testing.T) {
	for _, requirement := range []string{"^7.4", "^8.0", "~8.0.2"} {
		t.Run(requirement, func(t *testing.T) {
			dir := writeLaravelApp(t, requirement)
			t.Chdir(dir)

			srcInfo, err := configureLaravel(dir, &ScannerConfig{})

			require.Error(t, err)
			assert.Nil(t, srcInfo)
			assert.Contains(t, err.Error(), "PHP 8.1")
		})
	}
}

func TestConfigureLaravelUsesGeneratorFrom81(t *testing.T) {
	for _, requirement := range []string{"^8.1", "^8.2", "^8.3"} {
		t.Run(requirement, func(t *testing.T) {
			dir := writeLaravelApp(t, requirement)
			t.Chdir(dir)

			srcInfo, err := configureLaravel(dir, &ScannerConfig{})

			require.NoError(t, err)
			require.NotNil(t, srcInfo)
			assert.Empty(t, srcInfo.Files, "no Dockerfile template should be written; the generator produces it")
			assert.NotNil(t, srcInfo.Callback)
		})
	}
}

// Without Composer there is no generator to install. With no Dockerfile, say
// what to install; with one, launch goes ahead on it instead of running a
// generator that was never installed.
func TestLaravelCallbackWithoutComposer(t *testing.T) {
	t.Run("no Dockerfile", func(t *testing.T) {
		t.Chdir(writeLaravelApp(t, "^8.2"))
		t.Setenv("PATH", t.TempDir())

		err := LaravelCallback("my-app", &SourceInfo{Family: "Laravel"}, nil, nil)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "composer isn't on your PATH")
	})

	t.Run("existing Dockerfile", func(t *testing.T) {
		t.Chdir(writeLaravelApp(t, "^8.2"))
		t.Setenv("PATH", t.TempDir())
		require.NoError(t, os.WriteFile("Dockerfile", []byte("FROM php:8.2\n"), 0o644))

		srcInfo := &SourceInfo{Family: "Laravel"}
		err := LaravelCallback("my-app", srcInfo, nil, nil)

		require.NoError(t, err)
		assert.Contains(t, srcInfo.DeployDocs, "fly deploy")
	})
}

// writeComposerLock writes a composer.lock whose packages require the given
// PHP constraints, the first under "packages" and the rest under "packages-dev".
func writeComposerLock(t *testing.T, dir string, phpRequirements ...string) {
	t.Helper()
	pkg := func(i int, php string) string {
		return fmt.Sprintf(`{"name": "vendor/pkg%d", "version": "1.0.0", "require": {"php": %q}}`, i, php)
	}
	var packages, packagesDev []string
	for i, php := range phpRequirements {
		if i == 0 {
			packages = append(packages, pkg(i, php))
		} else {
			packagesDev = append(packagesDev, pkg(i, php))
		}
	}
	lock := fmt.Sprintf(`{"packages": [%s], "packages-dev": [%s]}`, strings.Join(packages, ","), strings.Join(packagesDev, ","))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "composer.lock"), []byte(lock), 0o644))
}

// composer.json gives the oldest PHP the app allows, but a lock resolved on a
// newer PHP can pin packages that need it. Composer can't add the Dockerfile
// generator to such a lock on the older PHP, so the plan has to use the lock's.
func TestConfigureLaravelPhpVersionFromComposerLock(t *testing.T) {
	cases := []struct {
		name     string
		require  string
		locked   []string
		expected string
	}{
		{"no lock", "^8.3", nil, "8.3"},
		{"lock needs newer PHP", "^8.3", []string{">=8.2", ">=8.4.1"}, "8.4"},
		{"lock within composer.json", "^8.3", []string{">=8.1", "^8.2"}, "8.3"},
		{"alternatives use their oldest", "^8.2", []string{"^7.4|^8.0", "^8.1 || ^8.3"}, "8.2"},
		{"no lower bound is ignored", "^8.2", []string{"*", "<9.0"}, "8.2"},
		{"lower bound after upper bound", "^8.2", []string{"<9.0 >=8.4"}, "8.4"},
		{"minor compared numerically", "^8.2", []string{">=8.10"}, "8.10"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := writeLaravelApp(t, tc.require)
			if tc.locked != nil {
				writeComposerLock(t, dir, tc.locked...)
			}
			t.Chdir(dir)

			srcInfo, err := configureLaravel(dir, &ScannerConfig{})

			require.NoError(t, err)
			assert.Equal(t, tc.expected, srcInfo.Runtime.Version)
			assert.Equal(t, tc.expected, srcInfo.BuildArgs["PHP_VERSION"])
		})
	}
}
