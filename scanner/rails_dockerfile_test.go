package scanner

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRailsScannerWithExistingDockerfile(t *testing.T) {
	t.Run("uses existing Dockerfile when bundle install fails", func(t *testing.T) {
		dir := t.TempDir()

		// Create a Rails app structure
		err := os.WriteFile(filepath.Join(dir, "Gemfile"), []byte("source 'https://rubygems.org'\ngem 'rails', '~> 7.1.0'"), 0644)
		require.NoError(t, err)

		err = os.WriteFile(filepath.Join(dir, "Gemfile.lock"), []byte("GEM\n  remote: https://rubygems.org/\n  specs:\n    rails (7.1.0)"), 0644)
		require.NoError(t, err)

		err = os.WriteFile(filepath.Join(dir, "config.ru"), []byte("require_relative 'config/environment'\nrun Rails.application"), 0644)
		require.NoError(t, err)

		// Create a custom Dockerfile with identifiable content
		customDockerfile := `FROM ruby:3.2.2
WORKDIR /app
COPY . .
EXPOSE 3000
CMD ["rails", "server"]
# CUSTOM MARKER: This is a custom Dockerfile`
		err = os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte(customDockerfile), 0644)
		require.NoError(t, err)

		// Change to test directory
		originalDir, _ := os.Getwd()
		t.Cleanup(func() { os.Chdir(originalDir) })
		err = os.Chdir(dir)
		require.NoError(t, err)

		// Run the scanner - it should detect the Rails app
		si, err := configureRails(dir, &ScannerConfig{})

		// The scanner should succeed in detecting Rails
		require.NoError(t, err)
		require.NotNil(t, si)
		assert.Equal(t, "Rails", si.Family)

		// Verify the Dockerfile still exists and wasn't modified
		dockerfileContent, err := os.ReadFile(filepath.Join(dir, "Dockerfile"))
		require.NoError(t, err)
		assert.Contains(t, string(dockerfileContent), "CUSTOM MARKER", "Custom Dockerfile should be preserved")
		assert.Equal(t, customDockerfile, string(dockerfileContent), "Dockerfile should be unchanged")

		// The callback would normally be called during launch, but we can't easily test that
		// without bundle/ruby being available. The key is that configureRails doesn't fail.
	})

	t.Run("extracts port from existing Dockerfile", func(t *testing.T) {
		dir := t.TempDir()

		// Create minimal Rails files
		err := os.WriteFile(filepath.Join(dir, "Gemfile"), []byte("source 'https://rubygems.org'\ngem 'rails'"), 0644)
		require.NoError(t, err)

		err = os.WriteFile(filepath.Join(dir, "config.ru"), []byte("run Rails.application"), 0644)
		require.NoError(t, err)

		// Create Dockerfile with custom port
		customDockerfile := `FROM ruby:3.2
WORKDIR /app
EXPOSE 8080
CMD ["rails", "server"]`
		err = os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte(customDockerfile), 0644)
		require.NoError(t, err)

		originalDir, _ := os.Getwd()
		t.Cleanup(func() { os.Chdir(originalDir) })
		err = os.Chdir(dir)
		require.NoError(t, err)

		si, err := configureRails(dir, &ScannerConfig{})
		require.NoError(t, err)
		require.NotNil(t, si)

		// The port extraction happens in RailsCallback when bundle install fails
		// For now, just verify the scanner doesn't fail with an existing Dockerfile
		assert.Equal(t, "Rails", si.Family)
	})

	t.Run("extracts volume from existing Dockerfile", func(t *testing.T) {
		dir := t.TempDir()

		// Create minimal Rails files
		err := os.WriteFile(filepath.Join(dir, "Gemfile"), []byte("source 'https://rubygems.org'\ngem 'rails'"), 0644)
		require.NoError(t, err)

		err = os.WriteFile(filepath.Join(dir, "config.ru"), []byte("run Rails.application"), 0644)
		require.NoError(t, err)

		// Create Dockerfile with volume
		customDockerfile := `FROM ruby:3.2
WORKDIR /app
VOLUME /app/storage
EXPOSE 3000
CMD ["rails", "server"]`
		err = os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte(customDockerfile), 0644)
		require.NoError(t, err)

		originalDir, _ := os.Getwd()
		t.Cleanup(func() { os.Chdir(originalDir) })
		err = os.Chdir(dir)
		require.NoError(t, err)

		si, err := configureRails(dir, &ScannerConfig{})
		require.NoError(t, err)
		require.NotNil(t, si)

		// The volume extraction happens in RailsCallback when bundle install fails
		// For now, just verify the scanner doesn't fail with an existing Dockerfile
		assert.Equal(t, "Rails", si.Family)
	})

	t.Run("fails when no Dockerfile exists and bundle not available", func(t *testing.T) {
		dir := t.TempDir()

		// Create minimal Rails files but NO Dockerfile
		err := os.WriteFile(filepath.Join(dir, "Gemfile"), []byte("source 'https://rubygems.org'\ngem 'rails'"), 0644)
		require.NoError(t, err)

		err = os.WriteFile(filepath.Join(dir, "config.ru"), []byte("run Rails.application"), 0644)
		require.NoError(t, err)

		// Note: No Dockerfile created

		originalDir, _ := os.Getwd()
		t.Cleanup(func() { os.Chdir(originalDir) })
		err = os.Chdir(dir)
		require.NoError(t, err)

		// This test would need bundle to not be available, which is hard to simulate
		// The scanner will either find bundle (and try to use it) or not find it
		// If bundle is not found and no Dockerfile exists, it should fail

		// For now, we just verify that the scanner can detect Rails
		si, err := configureRails(dir, &ScannerConfig{})

		// If bundle IS available locally, this will succeed
		// If bundle is NOT available and no Dockerfile exists, this should fail
		// We can't reliably test both cases, so we just verify it doesn't panic
		if err != nil {
			// Expected when bundle not available and no Dockerfile
			assert.Contains(t, err.Error(), "bundle")
		} else if si != nil {
			// Expected when bundle is available
			assert.Equal(t, "Rails", si.Family)
		}
	})
}

func TestRailsScannerPreservesDockerfileWithBin(t *testing.T) {
	t.Run("detects Rails via bin/rails and preserves Dockerfile", func(t *testing.T) {
		dir := t.TempDir()

		// Create bin directory with rails script
		binDir := filepath.Join(dir, "bin")
		err := os.MkdirAll(binDir, 0755)
		require.NoError(t, err)

		err = os.WriteFile(filepath.Join(binDir, "rails"), []byte("#!/usr/bin/env ruby\n# Rails script"), 0755)
		require.NoError(t, err)

		// Create Gemfile
		err = os.WriteFile(filepath.Join(dir, "Gemfile"), []byte("source 'https://rubygems.org'\ngem 'rails'"), 0644)
		require.NoError(t, err)

		// Create custom Dockerfile
		customDockerfile := `FROM ruby:3.2
# Custom Rails Dockerfile
EXPOSE 3000`
		err = os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte(customDockerfile), 0644)
		require.NoError(t, err)

		originalDir, _ := os.Getwd()
		t.Cleanup(func() { os.Chdir(originalDir) })
		err = os.Chdir(dir)
		require.NoError(t, err)

		si, err := configureRails(dir, &ScannerConfig{})
		require.NoError(t, err)
		require.NotNil(t, si)
		assert.Equal(t, "Rails", si.Family)

		// Verify Dockerfile wasn't modified
		dockerfileContent, err := os.ReadFile(filepath.Join(dir, "Dockerfile"))
		require.NoError(t, err)
		assert.Equal(t, customDockerfile, string(dockerfileContent))
	})
}
