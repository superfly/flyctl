package scanner

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadProcfile(t *testing.T) {
	dir := t.TempDir()
	assert.Nil(t, readProcfile(dir))

	writeFiles(t, dir, map[string]string{"Procfile": "# comment\nrelease: python migrate.py\n\nworker:  python bot.py \nnot a process\n"})
	assert.Equal(t, map[string]string{"release": "python migrate.py", "worker": "python bot.py"}, readProcfile(dir))
}

// The Python buildpack fallback (requirements.txt without a supported
// framework) used to configure an [http_service] on 8080 even for apps whose
// Procfile only runs a worker, like a Telegram bot.
func TestConfigurePythonProcfile(t *testing.T) {
	for _, tc := range []struct {
		name          string
		procfile      string
		wantPort      int
		wantNoHttp    bool
		wantGenerated bool
	}{
		{name: "worker only", procfile: "worker: python bot.py\n", wantNoHttp: true},
		{name: "worker and clock", procfile: "worker: python bot.py\nclock: python clock.py\n", wantNoHttp: true},
		{name: "web", procfile: "web: gunicorn app:app\n", wantPort: 8080},
		{name: "web and worker", procfile: "web: gunicorn app:app\nworker: celery -A app worker\n", wantPort: 8080},
		{name: "no Procfile", wantPort: 8080, wantGenerated: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			files := map[string]string{"requirements.txt": "pyTelegramBotAPI\nPillow\n"}
			if tc.procfile != "" {
				files["Procfile"] = tc.procfile
			}
			writeFiles(t, dir, files)
			t.Chdir(dir)

			si, err := configurePython(dir, nil)
			require.NoError(t, err)
			require.NotNil(t, si)

			assert.Equal(t, "Python", si.Family)
			assert.Equal(t, tc.wantPort, si.Port)
			assert.Equal(t, tc.wantNoHttp, si.NoHttpService)
			if tc.wantNoHttp {
				assert.NotContains(t, si.Env, "PORT")
			} else {
				assert.Equal(t, "8080", si.Env["PORT"])
			}

			generated := false
			for _, f := range si.Files {
				if f.Path == "Procfile" {
					generated = true
				}
			}
			assert.Equal(t, tc.wantGenerated, generated, "generates a Procfile")
		})
	}
}

func writeFiles(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for name, contents := range files {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(contents), 0o644))
	}
}
