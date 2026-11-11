package scanner

import (
	"os"
	"path/filepath"
	"runtime"
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

// writeFakeLaravelGenerator installs a vendor/bin/dockerfile-laravel that
// records its args and writes a Dockerfile, and returns the args file.
func writeFakeLaravelGenerator(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake generator is a shell script")
	}

	composer := `{"require": {"php": "^8.2"}, "require-dev": {"fly-apps/dockerfile-laravel": "*"}}`
	require.NoError(t, os.WriteFile("composer.json", []byte(composer), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join("vendor", "bin"), 0o755))

	argsFile := filepath.Join(t.TempDir(), "args")
	script := "#!/bin/sh\necho \"$@\" > " + argsFile + "\necho 'FROM php:8.2' > Dockerfile\n"
	require.NoError(t, os.WriteFile(filepath.Join("vendor", "bin", "dockerfile-laravel"), []byte(script), 0o755))

	return argsFile
}

func TestLaravelCallbackSkipsExistingFilesWhenNonInteractive(t *testing.T) {
	for _, tc := range []struct {
		name        string
		interactive bool
		wantSkip    bool
	}{
		{"non-interactive", false, true},
		{"interactive", true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Chdir(writeLaravelApp(t, "^8.2"))
			argsFile := writeFakeLaravelGenerator(t)

			err := LaravelCallback("my-app", &SourceInfo{Family: "Laravel", Interactive: tc.interactive}, nil, nil)
			require.NoError(t, err)

			args, err := os.ReadFile(argsFile)
			require.NoError(t, err)
			if tc.wantSkip {
				assert.Contains(t, strings.Fields(string(args)), "--skip", "with no one to answer, the generator must keep existing files")
			} else {
				assert.NotContains(t, strings.Fields(string(args)), "--skip", "a person at the terminal gets asked as before")
			}
		})
	}
}
