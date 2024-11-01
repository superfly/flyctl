package scanner

import (
	"fmt"
	"path/filepath"

	"github.com/superfly/flyctl/internal/command/launch/plan"
)

func configureDeno(sourceDir string, config *ScannerConfig) (*SourceInfo, error) {
	if !checksPass(
		sourceDir,
		// default config files: https://deno.land/manual@v1.35.2/getting_started/configuration_file
		fileExists("deno.json", "deno.jsonc"),
		// deno.land and denopkg.com imports
		dirContains("*.ts", `"https?://deno\.land/.*"`, `"https?://denopkg\.com/.*"`, `import "(.*)\.tsx{0,}"`, `from "npm:.*"`, `from "jsr:.*"`, `Deno\.serve\(.*`, `Deno\.listen\(.*`),
	) {
		return nil, nil
	}

	var entrypoint string

	for _, path := range []string{"index.ts", "app.ts", "server.ts"} {
		if absFileExists(filepath.Join(sourceDir, path)) {
			entrypoint = path
			break
		}
	}

	s := &SourceInfo{
		Files:  templates("templates/deno"),
		Family: "Deno",
		Port:   8080,
		Processes: map[string]string{
			"app": fmt.Sprintf("run -A ./%s", entrypoint),
		},
		Env: map[string]string{
			"PORT": "8080",
		},
		Runtime: plan.RuntimeStruct{Language: "deno"},
	}

	return s, nil
}
