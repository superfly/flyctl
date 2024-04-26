package scanner

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
)

const defaultPort = 8080

var portRegex = regexp.MustCompile(`(?m)^EXPOSE\s+(?P<port>\d+)`)

func configureDockerfile(sourceDir string, config *ScannerConfig) (*SourceInfo, error) {
	return ScanDockerfile(filepath.Join(sourceDir, "Dockerfile"), config)
}

func ScanDockerfile(dockerfilePath string, config *ScannerConfig) (*SourceInfo, error) {
	if !absFileExists(dockerfilePath) {
		return nil, nil
	}

	var portFromDockerfile int

	s := &SourceInfo{
		DockerfilePath: dockerfilePath,
		Family:         "Dockerfile",
		Port:           config.ExistingPort,
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

	// extract volume - handle both plain string and JSON format, but only allow one path
	re := regexp.MustCompile(`(?m)^VOLUME\s+(\[\s*")?(\/[\w\/]*?(\w+))("\s*\])?\s*$`)
	m = re.FindStringSubmatch(string(dockerfile))

	if len(m) > 0 {
		s.Volumes = []Volume{
			{
				Source:      m[3], // last part of path
				Destination: m[2], // full path
			},
		}
	}

	return s, nil
}
