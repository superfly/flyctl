package scanner

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCheckExistingDockerfile(t *testing.T) {
	t.Run("returns true when Dockerfile exists", func(t *testing.T) {
		dir := t.TempDir()
		err := os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte("FROM node:18"), 0644)
		require.NoError(t, err)

		exists, path := checkExistingDockerfile(dir, "TestFramework")
		assert.True(t, exists)
		assert.Equal(t, "Dockerfile", path)
	})

	t.Run("returns false when Dockerfile does not exist", func(t *testing.T) {
		dir := t.TempDir()

		exists, path := checkExistingDockerfile(dir, "TestFramework")
		assert.False(t, exists)
		assert.Equal(t, "", path)
	})
}

func TestFilterDockerfile(t *testing.T) {
	t.Run("removes Dockerfile from file list", func(t *testing.T) {
		files := []SourceFile{
			{Path: "Dockerfile", Contents: []byte("FROM node:18")},
			{Path: ".dockerignore", Contents: []byte("node_modules")},
			{Path: "docker-entrypoint", Contents: []byte("#!/bin/sh")},
		}

		filtered := filterDockerfile(files)

		assert.Len(t, filtered, 2)
		assert.Equal(t, ".dockerignore", filtered[0].Path)
		assert.Equal(t, "docker-entrypoint", filtered[1].Path)
	})

	t.Run("returns empty list when only Dockerfile exists", func(t *testing.T) {
		files := []SourceFile{
			{Path: "Dockerfile", Contents: []byte("FROM node:18")},
		}

		filtered := filterDockerfile(files)

		assert.Len(t, filtered, 0)
	})

	t.Run("returns all files when no Dockerfile exists", func(t *testing.T) {
		files := []SourceFile{
			{Path: ".dockerignore", Contents: []byte("node_modules")},
			{Path: "docker-entrypoint", Contents: []byte("#!/bin/sh")},
		}

		filtered := filterDockerfile(files)

		assert.Len(t, filtered, 2)
		assert.Equal(t, files, filtered)
	})

	t.Run("handles empty list", func(t *testing.T) {
		files := []SourceFile{}

		filtered := filterDockerfile(files)

		assert.Len(t, filtered, 0)
	})
}
