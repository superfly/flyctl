package scanner

import (
	"path/filepath"

	"github.com/superfly/flyctl/helpers"
)

func configureStatic(sourceDir string, config *ScannerConfig) (*SourceInfo, error) {
	// No index.html detected, move on
	if !helpers.FileExists(filepath.Join(sourceDir, "index.html")) {
		return nil, nil
	}

	s := &SourceInfo{
		Family: "Static",
		Port:   8080,
		Files:  templates("templates/static"),
	}

	return s, nil
}
