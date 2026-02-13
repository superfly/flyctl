package scanner

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRailsObjectStorageDetection(t *testing.T) {
	t.Run("migration plus local production storage does not request object storage", func(t *testing.T) {
		dir := t.TempDir()
		writeRailsScannerFixture(t, dir, "config.active_storage.service = :local\n")
		writeActiveStorageMigration(t, dir)

		originalDir, err := os.Getwd()
		require.NoError(t, err)
		defer os.Chdir(originalDir)
		require.NoError(t, os.Chdir(dir))

		si, err := configureRails(dir, &ScannerConfig{})
		require.NoError(t, err)
		require.NotNil(t, si)
		assert.False(t, si.ObjectStorageDesired)
	})

	t.Run("migration plus non-local production storage requests object storage", func(t *testing.T) {
		dir := t.TempDir()
		writeRailsScannerFixture(t, dir, "config.active_storage.service = :amazon\n")
		writeActiveStorageMigration(t, dir)

		originalDir, err := os.Getwd()
		require.NoError(t, err)
		defer os.Chdir(originalDir)
		require.NoError(t, os.Chdir(dir))

		si, err := configureRails(dir, &ScannerConfig{})
		require.NoError(t, err)
		require.NotNil(t, si)
		assert.True(t, si.ObjectStorageDesired)
	})

	t.Run("sqlite package in Dockerfile alone does not request object storage", func(t *testing.T) {
		dir := t.TempDir()
		writeRailsScannerFixture(t, dir, "config.active_storage.service = :local\n")

		require.NoError(t, os.WriteFile(
			filepath.Join(dir, "Dockerfile"),
			[]byte("FROM ruby:3.3-slim\nRUN apt-get update -qq && apt-get install --no-install-recommends -y sqlite3\nEXPOSE 80\n"),
			0644,
		))

		originalDir, err := os.Getwd()
		require.NoError(t, err)
		defer os.Chdir(originalDir)
		require.NoError(t, os.Chdir(dir))

		si, err := configureRails(dir, &ScannerConfig{})
		require.NoError(t, err)
		require.NotNil(t, si)
		assert.False(t, si.ObjectStorageDesired)
	})

	t.Run("commented S3 entries in storage.yml do not request object storage", func(t *testing.T) {
		dir := t.TempDir()
		writeRailsScannerFixture(t, dir, "config.active_storage.service = :local\n")

		require.NoError(t, os.WriteFile(
			filepath.Join(dir, "config", "storage.yml"),
			[]byte("local:\n  service: Disk\n\n# amazon:\n#   service: S3\n#   bucket: foo\n"),
			0644,
		))

		originalDir, err := os.Getwd()
		require.NoError(t, err)
		defer os.Chdir(originalDir)
		require.NoError(t, os.Chdir(dir))

		si, err := configureRails(dir, &ScannerConfig{})
		require.NoError(t, err)
		require.NotNil(t, si)
		assert.False(t, si.ObjectStorageDesired)
	})

	t.Run("active S3 service in storage.yml requests object storage", func(t *testing.T) {
		dir := t.TempDir()
		writeRailsScannerFixture(t, dir, "config.active_storage.service = :local\n")

		require.NoError(t, os.WriteFile(
			filepath.Join(dir, "config", "storage.yml"),
			[]byte("amazon:\n  service: S3\n  bucket: foo\n"),
			0644,
		))

		originalDir, err := os.Getwd()
		require.NoError(t, err)
		defer os.Chdir(originalDir)
		require.NoError(t, os.Chdir(dir))

		si, err := configureRails(dir, &ScannerConfig{})
		require.NoError(t, err)
		require.NotNil(t, si)
		assert.True(t, si.ObjectStorageDesired)
	})
}

func writeRailsScannerFixture(t *testing.T, dir string, prodStorageLine string) {
	t.Helper()

	require.NoError(t, os.MkdirAll(filepath.Join(dir, "config", "environments"), 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "db", "migrate"), 0755))

	require.NoError(t, os.WriteFile(
		filepath.Join(dir, "Gemfile"),
		[]byte("source 'https://rubygems.org'\ngem 'rails', '~> 8.0'\n"),
		0644,
	))
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, "Gemfile.lock"),
		[]byte("GEM\n  specs:\n    rails (8.0.0)\n"),
		0644,
	))
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, "config.ru"),
		[]byte("require_relative 'config/environment'\nrun Rails.application\n"),
		0644,
	))

	require.NoError(t, os.WriteFile(
		filepath.Join(dir, "Dockerfile"),
		[]byte("FROM ruby:3.3-slim\nEXPOSE 80\n"),
		0644,
	))

	require.NoError(t, os.WriteFile(
		filepath.Join(dir, "config", "storage.yml"),
		[]byte("local:\n  service: Disk\n"),
		0644,
	))

	for _, env := range []string{"development", "test"} {
		require.NoError(t, os.WriteFile(
			filepath.Join(dir, "config", "environments", env+".rb"),
			[]byte("Rails.application.configure do\n  config.active_storage.service = :local\nend\n"),
			0644,
		))
	}

	require.NoError(t, os.WriteFile(
		filepath.Join(dir, "config", "environments", "production.rb"),
		[]byte("Rails.application.configure do\n  "+prodStorageLine+"end\n"),
		0644,
	))
}

func writeActiveStorageMigration(t *testing.T, dir string) {
	t.Helper()

	require.NoError(t, os.WriteFile(
		filepath.Join(dir, "db", "migrate", "20250101000000_create_active_storage_tables.rb"),
		[]byte("class CreateActiveStorageTables < ActiveRecord::Migration[7.1]\n  def change\n    create_table :active_storage_attachments do |t|\n      t.string :name\n    end\n  end\nend\n"),
		0644,
	))
}
