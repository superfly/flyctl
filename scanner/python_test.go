package scanner

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Having no dependencies in one file isn't an error: they may be in another
// (a pyproject.toml that only configures tools, a Pipfile with only
// dev-packages, Poetry 2 listing them under [project]). An empty
// requirements.txt gets the generic Python setup, like requirements that name
// no framework fly launch knows.
func TestConfigurePythonWithoutDependencies(t *testing.T) {
	for _, tc := range []struct {
		name       string
		files      map[string]string
		wantFamily string // "" for not claimed as Python
		wantPoetry bool
	}{
		{
			name:       "empty requirements.txt",
			files:      map[string]string{"requirements.txt": ""},
			wantFamily: "Python",
		},
		{
			name: "tool-only pyproject.toml and requirements.txt",
			files: map[string]string{
				"pyproject.toml":   "[tool.ruff]\nline-length = 100\n",
				"requirements.txt": "flask==3.0.0\n",
			},
			wantFamily: "Flask",
		},
		{
			name: "Pipfile with only dev-packages and requirements.txt",
			files: map[string]string{
				"Pipfile":          "[dev-packages]\npytest = \"*\"\n",
				"requirements.txt": "flask==3.0.0\n",
			},
			wantFamily: "Flask",
		},
		{
			name: "Poetry 2 with dependencies under [project]",
			files: map[string]string{
				"poetry.lock":    "",
				"pyproject.toml": "[project]\nname = \"app\"\ndependencies = [\"flask>=3\"]\nrequires-python = \">=3.12\"\n\n[tool.poetry]\npackage-mode = false\n",
			},
			wantFamily: "Flask",
			wantPoetry: true,
		},
		{
			name: "Poetry 2 with only python under [tool.poetry.dependencies]",
			files: map[string]string{
				"poetry.lock":    "",
				"pyproject.toml": "[project]\nname = \"app\"\ndependencies = [\"fastapi\"]\n\n[tool.poetry.dependencies]\npython = \"^3.12\"\n",
			},
			wantFamily: "FastAPI",
			wantPoetry: true,
		},
		{
			name: "Poetry without a python dependency",
			files: map[string]string{
				"poetry.lock":    "",
				"pyproject.toml": "[tool.poetry]\nname = \"app\"\n\n[tool.poetry.dependencies]\nflask = \"^3.0\"\n",
			},
			wantFamily: "Flask",
			wantPoetry: true,
		},
		{
			// Left for the Rust scanner, as on master.
			name: "maturin project",
			files: map[string]string{
				"Cargo.toml":     "[package]\nname = \"ext\"\n",
				"pyproject.toml": "[project]\nname = \"ext\"\ndependencies = [\"cffi\"]\n",
			},
		},
		{
			name:  "tool-only pyproject.toml alone",
			files: map[string]string{"pyproject.toml": "[tool.ruff]\nline-length = 100\n"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Chdir(t.TempDir())
			for name, contents := range tc.files {
				require.NoError(t, os.WriteFile(name, []byte(contents), 0o644))
			}

			srcInfo, err := configurePython(".", &ScannerConfig{})

			require.NoError(t, err)
			if tc.wantFamily == "" {
				assert.Nil(t, srcInfo)

				return
			}
			require.NotNil(t, srcInfo)
			assert.Equal(t, tc.wantFamily, srcInfo.Family)
			if tc.wantPoetry {
				var dockerfile string
				for _, f := range srcInfo.Files {
					if f.Path == "Dockerfile" {
						dockerfile = string(f.Contents)
					}
				}
				assert.Contains(t, dockerfile, "poetry install", "installs from poetry.lock")
			}
		})
	}
}
