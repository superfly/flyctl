package scanner

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPythonScannerPreservesDockerfile(t *testing.T) {
	t.Run("FastAPI uses existing Dockerfile and generates .dockerignore", func(t *testing.T) {
		dir := t.TempDir()

		// Create a custom Dockerfile
		customDockerfile := "FROM python:3.11\nRUN echo 'custom fastapi dockerfile'"
		err := os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte(customDockerfile), 0644)
		require.NoError(t, err)

		// Create requirements.txt with FastAPI
		requirements := "fastapi>=0.100.0\nuvicorn[standard]"
		err = os.WriteFile(filepath.Join(dir, "requirements.txt"), []byte(requirements), 0644)
		require.NoError(t, err)

		// Change to the test directory so the scanner can find the files
		originalDir, _ := os.Getwd()
		defer os.Chdir(originalDir)
		err = os.Chdir(dir)
		require.NoError(t, err)

		si, err := configRequirements(dir, &ScannerConfig{})
		require.NoError(t, err)
		require.NotNil(t, si)

		// Should use the existing Dockerfile
		assert.Equal(t, "Dockerfile", si.DockerfilePath)
		assert.Equal(t, "FastAPI", si.Family)

		// Should still generate .dockerignore
		assert.Greater(t, len(si.Files), 0, "Should generate non-Dockerfile files")

		// Verify that Dockerfile is NOT in the Files list
		for _, file := range si.Files {
			assert.NotEqual(t, "Dockerfile", file.Path, "Should not generate Dockerfile when one exists")
		}

		// Verify .dockerignore IS in the Files list
		hasDockerignore := false
		for _, file := range si.Files {
			if file.Path == ".dockerignore" {
				hasDockerignore = true
				break
			}
		}
		assert.True(t, hasDockerignore, "Should generate .dockerignore even when Dockerfile exists")
	})

	t.Run("Flask uses existing Dockerfile and generates .dockerignore", func(t *testing.T) {
		dir := t.TempDir()

		// Create a custom Dockerfile
		customDockerfile := "FROM python:3.11\nRUN echo 'custom flask dockerfile'"
		err := os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte(customDockerfile), 0644)
		require.NoError(t, err)

		// Create requirements.txt with Flask
		requirements := "flask>=2.0.0\ngunicorn"
		err = os.WriteFile(filepath.Join(dir, "requirements.txt"), []byte(requirements), 0644)
		require.NoError(t, err)

		// Change to the test directory
		originalDir, _ := os.Getwd()
		defer os.Chdir(originalDir)
		err = os.Chdir(dir)
		require.NoError(t, err)

		si, err := configRequirements(dir, &ScannerConfig{})
		require.NoError(t, err)
		require.NotNil(t, si)

		// Should use the existing Dockerfile
		assert.Equal(t, "Dockerfile", si.DockerfilePath)
		assert.Equal(t, "Flask", si.Family)

		// Should still generate .dockerignore
		assert.Greater(t, len(si.Files), 0, "Should generate non-Dockerfile files")

		// Verify that Dockerfile is NOT in the Files list
		for _, file := range si.Files {
			assert.NotEqual(t, "Dockerfile", file.Path, "Should not generate Dockerfile when one exists")
		}
	})

	t.Run("generates all files when no Dockerfile exists", func(t *testing.T) {
		dir := t.TempDir()

		// Create requirements.txt with FastAPI (no Dockerfile)
		requirements := "fastapi>=0.100.0\nuvicorn[standard]"
		err := os.WriteFile(filepath.Join(dir, "requirements.txt"), []byte(requirements), 0644)
		require.NoError(t, err)

		// Change to the test directory
		originalDir, _ := os.Getwd()
		defer os.Chdir(originalDir)
		err = os.Chdir(dir)
		require.NoError(t, err)

		si, err := configRequirements(dir, &ScannerConfig{})
		require.NoError(t, err)
		require.NotNil(t, si)

		// Should NOT have DockerfilePath set
		assert.Equal(t, "", si.DockerfilePath)

		// Should generate all files including Dockerfile
		hasDockerfile := false
		hasDockerignore := false
		for _, file := range si.Files {
			if file.Path == "Dockerfile" {
				hasDockerfile = true
			}
			if file.Path == ".dockerignore" {
				hasDockerignore = true
			}
		}
		assert.True(t, hasDockerfile, "Should generate Dockerfile when none exists")
		assert.True(t, hasDockerignore, "Should generate .dockerignore")
	})

	t.Run("Poetry FastAPI uses existing Dockerfile", func(t *testing.T) {
		dir := t.TempDir()

		// Create a custom Dockerfile
		customDockerfile := "FROM python:3.11\nRUN echo 'custom poetry dockerfile'"
		err := os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte(customDockerfile), 0644)
		require.NoError(t, err)

		// Create pyproject.toml for Poetry with FastAPI
		pyprojectContent := `[tool.poetry]
name = "test-app"
version = "0.1.0"

[tool.poetry.dependencies]
python = "^3.11"
fastapi = "^0.100.0"
`
		err = os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte(pyprojectContent), 0644)
		require.NoError(t, err)

		// Create poetry.lock
		err = os.WriteFile(filepath.Join(dir, "poetry.lock"), []byte(""), 0644)
		require.NoError(t, err)

		// Change to the test directory
		originalDir, _ := os.Getwd()
		defer os.Chdir(originalDir)
		err = os.Chdir(dir)
		require.NoError(t, err)

		si, err := configPoetry(dir, &ScannerConfig{})
		require.NoError(t, err)
		require.NotNil(t, si)

		// Should use the existing Dockerfile
		assert.Equal(t, "Dockerfile", si.DockerfilePath)
		assert.Equal(t, "FastAPI", si.Family)

		// Verify that Dockerfile is NOT in the Files list
		for _, file := range si.Files {
			assert.NotEqual(t, "Dockerfile", file.Path, "Should not generate Dockerfile when one exists")
		}
	})
}
