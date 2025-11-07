package scanner

import (
	"fmt"
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
	}

	hasDockerfile := checksPass(sourceDir, fileExists("Dockerfile"))
	if hasDockerfile {
		s.DockerfilePath = "Dockerfile"
		fmt.Printf("Detected existing Dockerfile, will use it for static site\n")
	} else {
		s.Files = templates("templates/static")
	}

	return s, nil
}
