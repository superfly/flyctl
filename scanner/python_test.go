package scanner

import (
	"os"
	"path/filepath"
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
			name: "Poetry 2 with extras for a [project] dependency",
			files: map[string]string{
				"poetry.lock":    "",
				"pyproject.toml": "[project]\nname = \"app\"\ndependencies = [\"fastapi\", \"uvicorn\"]\n\n[tool.poetry.dependencies]\nuvicorn = {extras = [\"standard\"]}\n",
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

func TestConfigurePythonFallbackProcfile(t *testing.T) {
	tests := []struct {
		name     string
		files    map[string]string
		procfile string // expected Procfile contents; "" means no Procfile is generated
	}{
		{
			// No supported framework and no gunicorn: the generated Procfile used
			// to run gunicorn, and the app crashed with
			// "bash: gunicorn: command not found".
			name:     "requirements without gunicorn or entrypoint",
			files:    map[string]string{"requirements.txt": "requests\nopenai\n"},
			procfile: "",
		},
		{
			name: "requirements with gunicorn",
			files: map[string]string{
				"requirements.txt": "requests\ngunicorn==23.0.0\n",
				"app.py":           "",
			},
			procfile: "# TODO: Modify this Procfile to fit your needs\nweb: gunicorn app:app\n",
		},
		{
			name: "app.py without gunicorn",
			files: map[string]string{
				"requirements.txt": "requests\n",
				"app.py":           "",
			},
			procfile: "# TODO: Modify this Procfile to fit your needs\nweb: python app.py\n",
		},
		{
			name: "main.py without gunicorn",
			files: map[string]string{
				"requirements.txt": "requests\n",
				"main.py":          "",
			},
			procfile: "# TODO: Modify this Procfile to fit your needs\nweb: python main.py\n",
		},
		{
			name: "gunicorn in setup.py",
			files: map[string]string{
				"setup.py": "install_requires=['gunicorn']\n",
			},
			procfile: "# TODO: Modify this Procfile to fit your needs\nweb: gunicorn app:app\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			for name, contents := range tt.files {
				require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(contents), 0o644))
			}
			t.Chdir(dir)

			si, err := configurePython(dir, nil)
			require.NoError(t, err)
			require.NotNil(t, si)
			assert.Equal(t, "paketobuildpacks/builder:base", si.Builder)

			var procfile *SourceFile
			for i := range si.Files {
				if si.Files[i].Path == "Procfile" {
					procfile = &si.Files[i]
				}
			}

			if tt.procfile == "" {
				assert.Nil(t, procfile, "should not generate a Procfile it can't back with an installed server")
				return
			}
			require.NotNil(t, procfile)
			assert.Equal(t, tt.procfile, string(procfile.Contents))
		})
	}
}
