package scanner

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
)

func configureDockerfile(sourceDir string, config *ScannerConfig) (*SourceInfo, error) {
	if !checksPass(sourceDir, fileExists("Dockerfile")) {
		return nil, nil
	}

	s := &SourceInfo{
		DockerfilePath: filepath.Join(sourceDir, "Dockerfile"),
		Family:         "Dockerfile",
	}

	// extract port number from EXPOSE statement
	dockerfile, err := os.ReadFile("Dockerfile")
	if err == nil {
		port := 8080
		re := regexp.MustCompile(`(?m)^EXPOSE\s+(?P<port>\d+)`)
		m := re.FindStringSubmatch(string(dockerfile))

		for i, name := range re.SubexpNames() {
			if len(m) > 0 && name == "port" {
				port, err = strconv.Atoi(m[i])
				if err != nil {
					panic(err)
				}
			}
		}
		s.Port = port
	}

	return s, nil
}
