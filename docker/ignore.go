package docker

import (
	"os"
	"path/filepath"

	"github.com/docker/docker/builder/dockerignore"
	"github.com/docker/docker/pkg/fileutils"
	"github.com/superfly/flyctl/terminal"
)

func readDockerignore(workingDir string) ([]string, error) {
	file, err := os.Open(filepath.Join(workingDir, ".dockerignore"))
	if os.IsNotExist(err) {
		return []string{}, nil
	} else if err != nil {
		terminal.Warn("Error reading dockerignore", err)
		return []string{}, nil
	}

	excludes, err := dockerignore.ReadAll(file)
	if err == nil {
		excludes = trimExcludes(excludes)
	}

	return excludes, err
}

func trimExcludes(excludes []string) []string {
	if match, _ := fileutils.Matches(".dockerignore", excludes); match {
		excludes = append(excludes, "!.dockerignore")
	}

	if match, _ := fileutils.Matches("Dockerfile", excludes); match {
		excludes = append(excludes, "![Dd]ockerfile")
	}

	return excludes
}
