//go:build !production

package buildinfo

import (
	"time"

	"github.com/superfly/flyctl/internal/version"
)

var (
	buildDate   = "<date>"
	environment = "development"
)

func loadBuildTime() (err error) {
	// Makefile sets proper values for buildDate but bare `go run .` doesn't
	if buildDate == "<date>" {
		buildDate = time.Now().Format(time.RFC3339)
	}
	cachedBuildTime, err = time.Parse(time.RFC3339, buildDate)
	return
}

func loadVersion() error {
	// Makefile sets proper values for branchName but bare `go run .` doesn't
	if branchName == "" {
		branchName = "dev"
	}
	cachedVersion = version.New(cachedBuildTime, branchName, int(cachedBuildTime.Unix()))
	return nil
}
