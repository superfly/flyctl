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

// stubCommands puts shell scripts for the given commands on PATH, in place of
// everything else. Each one appends its name and arguments to the returned log.
func stubCommands(t *testing.T, commands map[string]string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("stub commands are shell scripts")
	}

	bin := t.TempDir()
	log := filepath.Join(t.TempDir(), "commands.log")
	for name, body := range commands {
		script := "#!/bin/sh\necho \"" + name + " $*\" >> " + log + "\n" + body + "\n"
		require.NoError(t, os.WriteFile(filepath.Join(bin, name), []byte(script), 0o755))
	}
	t.Setenv("PATH", bin)

	return log
}

func readLog(t *testing.T, log string) []string {
	t.Helper()
	data, err := os.ReadFile(log)
	if os.IsNotExist(err) {
		return nil
	}
	require.NoError(t, err)

	return strings.Split(strings.TrimSpace(string(data)), "\n")
}

const installGenerator = "/bin/mkdir -p vendor/bin && /usr/bin/touch vendor/bin/dockerfile-laravel"

func TestLaravelCallbackInstallsAndRunsGenerator(t *testing.T) {
	t.Chdir(writeLaravelApp(t, "^8.2"))
	log := stubCommands(t, map[string]string{"php": "", "composer": installGenerator})

	require.NoError(t, LaravelCallback("app", &SourceInfo{Family: "Laravel"}, nil, nil))

	assert.Equal(t, []string{
		"composer require --dev fly-apps/dockerfile-laravel",
		"php " + filepath.Join("vendor", "bin", "dockerfile-laravel") + " generate",
	}, readLog(t, log))
}

func TestLaravelCallbackKeepsExistingDockerfileWithoutComposer(t *testing.T) {
	dir := writeLaravelApp(t, "^8.2")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte("FROM scratch\n"), 0o644))
	t.Chdir(dir)
	log := stubCommands(t, map[string]string{"docker": ""})

	require.NoError(t, LaravelCallback("app", &SourceInfo{Family: "Laravel"}, nil, nil))

	assert.Empty(t, readLog(t, log))
}

func TestLaravelCallbackGeneratesInDockerWithoutComposer(t *testing.T) {
	t.Chdir(writeLaravelApp(t, "^8.2"))
	log := stubCommands(t, map[string]string{"docker": ""})

	require.NoError(t, LaravelCallback("app", &SourceInfo{Family: "Laravel"}, nil, []string{"--octane=frankenphp"}))

	calls := readLog(t, log)
	require.Len(t, calls, 2)
	assert.Contains(t, calls[0], "composer:2 composer require --dev --no-interaction --ignore-platform-reqs fly-apps/dockerfile-laravel")
	assert.Contains(t, calls[1], "composer:2 php vendor/bin/dockerfile-laravel generate --octane=frankenphp")
}

func TestLaravelCallbackExplainsMissingTools(t *testing.T) {
	t.Chdir(writeLaravelApp(t, "^8.2"))
	stubCommands(t, map[string]string{})

	err := LaravelCallback("app", &SourceInfo{Family: "Laravel"}, nil, nil)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "Install Composer")
}

func TestLaravelCallbackReportsFailedInstall(t *testing.T) {
	t.Chdir(writeLaravelApp(t, "^8.2"))
	log := stubCommands(t, map[string]string{"php": "", "composer": "exit 2", "docker": ""})

	err := LaravelCallback("app", &SourceInfo{Family: "Laravel"}, nil, nil)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to install fly-apps/dockerfile-laravel")
	assert.Equal(t, []string{"composer require --dev fly-apps/dockerfile-laravel"}, readLog(t, log))
}
