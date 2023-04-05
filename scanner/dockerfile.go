package scanner

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
)

const defaultPort = 8080

var portRegex = regexp.MustCompile(`(?m)^EXPOSE\s+(?P<port>\d+)`)

func configureDockerfile (sourceDir string, config *ScannerConfig) (*SourceInfo, error) {
	if !checksPass(sourceDir, fileExists("Dockerfile")) {
		return nil, nil
	}

	var portFromDockerfile int

	s := &SourceInfo{
		DockerfilePath: filepath.Join(sourceDir, "Dockerfile"),
		Family:         "Dockerfile",
		Port : config.ExistingPort,
	}

	dockerfile, err := os.ReadFile(s.DockerfilePath)
	if err != nil {
		// just maintaining existing behaviour from old code.
		return s, nil
	}

	m := portRegex.FindStringSubmatch(string(dockerfile))

	for i, name := range portRegex.SubexpNames() {
		if len(m) > 0 && name == "port" {
			portFromDockerfile, err = strconv.Atoi(m[i])
			if err != nil {
				continue
			}
		}
	}

	if portFromDockerfile != 0 {
		s.Port = portFromDockerfile
	}

	if s.Port == 0 {
		s.Port = defaultPort
	}

	return s, nil
}
