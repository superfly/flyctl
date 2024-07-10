package scanner

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

func configureMeteor(sourceDir string, config *ScannerConfig) (*SourceInfo, error) {
	if !checksPass(sourceDir, dirContains("package.json", "\".meteor\"")) {
		return nil, nil
	}

	releaseFilePath := sourceDir + "/.meteor/release"
	releaseContent, err := os.ReadFile(releaseFilePath)
	if err != nil {
		return nil, err
	}

	releaseString := strings.TrimSpace(string(releaseContent))
	parts := strings.Split(releaseString, "@")
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid release format in %s", releaseFilePath)
	}
	meteorVersion := parts[1]

	versionParts := strings.Split(meteorVersion, ".")
	if len(versionParts) == 0 {
		return nil, fmt.Errorf("invalid version format in %s", releaseFilePath)
	}
	majorVersion, err := strconv.Atoi(strings.TrimPrefix(versionParts[0], "v"))
	if err != nil {
		return nil, fmt.Errorf("invalid major version in %s", releaseFilePath)
	}
	if majorVersion < 3 {
		return nil, fmt.Errorf("major version must be 3 or higher in %s", releaseFilePath)
	}

	env := map[string]string{
		"PORT": "3000",
	}

	s := &SourceInfo{
		Family:       "Meteor",
		Port:         3000,
		SkipDatabase: true,
		Env:          env,
	}

	s.Files = templates("templates/meteor")

	s.Env["meteorVersion"] = meteorVersion

	return s, nil
}
