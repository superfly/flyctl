package scanner

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfigurePythonFallbackProcfile(t *testing.T) {
	tests := []struct {
		name     string
		files    map[string]string
		procfile string // expected Procfile contents; "" means no Procfile is generated
	}{
		{
			// Deployment 2277002: no supported framework and no gunicorn, yet the
			// generated Procfile ran gunicorn and the app crashed with
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
