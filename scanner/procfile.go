package scanner

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var procfileLine = regexp.MustCompile(`^([A-Za-z0-9_-]+):\s*(.+)$`)

// readProcfile returns the process types declared in sourceDir's Procfile,
// mapped to their commands, or nil when there is no Procfile.
func readProcfile(sourceDir string) map[string]string {
	contents, err := os.ReadFile(filepath.Join(sourceDir, "Procfile"))
	if err != nil {
		return nil
	}

	processes := map[string]string{}
	for _, line := range strings.Split(string(contents), "\n") {
		if m := procfileLine.FindStringSubmatch(strings.TrimSpace(line)); m != nil {
			processes[m[1]] = strings.TrimSpace(m[2])
		}
	}

	return processes
}

// procfileHasNoWeb reports whether sourceDir has a Procfile that declares no
// web process, e.g. only `worker: python bot.py`. Buildpacks then run one of
// the other processes, which won't listen for HTTP.
func procfileHasNoWeb(sourceDir string) bool {
	processes := readProcfile(sourceDir)
	if processes == nil {
		return false
	}
	_, hasWeb := processes["web"]

	return !hasWeb
}
