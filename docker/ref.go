package docker

import (
	"os"
	"path"
	"regexp"
	"strings"

	"github.com/superfly/flyctl/helpers"
)

var remotePattern = regexp.MustCompile(`^[\w\d-]+\.`)

func IsRemoteImageReference(imageName string) bool {
	if strings.HasPrefix(imageName, ".") {
		return false
	}

	return remotePattern.MatchString(imageName)
}

func IsDockerfilePath(imageName string) bool {
	cwd, err := os.Getwd()
	if err != nil {
		return false
	}

	maybePath := path.Join(cwd, imageName)

	return helpers.FileExists(maybePath)
}

func IsDirContainingDockerfile(imageName string) bool {
	cwd, err := os.Getwd()
	if err != nil {
		return false
	}

	maybePath := path.Join(cwd, imageName, "Dockerfile")

	return helpers.FileExists(maybePath)
}
