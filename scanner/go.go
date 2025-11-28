package scanner

import (
	"fmt"
	"os"

	"github.com/superfly/flyctl/internal/command/launch/plan"
	"github.com/superfly/flyctl/terminal"
	"golang.org/x/mod/modfile"
)

func configureGo(sourceDir string, config *ScannerConfig) (*SourceInfo, error) {
	if !checksPass(sourceDir, fileExists("go.mod")) {
		return nil, nil
	}

	vars := make(map[string]interface{})

	var skipDeploy bool

	if !absFileExists("go.sum") {
		vars["skipGoSum"] = true
		skipDeploy = true
	}

	gomod, parseErr := parseModfile()

	version := "1"
	if parseErr != nil {
		terminal.Warnf("go.mod appears to be invalid, the next deployment may fail: %v", parseErr)
	} else if gomod.Go.Version != "" {
		version = gomod.Go.Version
	}

	s := &SourceInfo{
		Files:  templatesExecute("templates/go", vars),
		Family: "Go",
		Port:   8080,
		Env: map[string]string{
			"PORT": "8080",
		},
		Runtime: plan.RuntimeStruct{Language: "go", Version: version},
		BuildArgs: map[string]string{
			"GO_VERSION": version,
		},
		SkipDeploy: skipDeploy,
	}

	return s, nil
}

func parseModfile() (*modfile.File, error) {
	dat, err := os.ReadFile("go.mod")
	if err != nil {
		return nil, fmt.Errorf("could not open go.mod: %w", err)
	}

	f, modErr := modfile.Parse("go.mod", dat, nil)

	if modErr != nil {
		return nil, fmt.Errorf("could not parse go.mod: %w", modErr)
	}

	return f, nil
}
