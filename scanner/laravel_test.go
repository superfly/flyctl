package scanner

import (
	"os"
	"path/filepath"
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
