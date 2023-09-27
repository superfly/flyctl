//go:build !production

package buildinfo

import (
	"time"

	"github.com/superfly/flyctl/internal/version"
)

var environment = "development"

func init() {

}

func loadBuildTime() error {
	cachedBuildTime = time.Now()
	return nil
}

func loadVersion() error {
	cachedVersion = version.Version{
		Major:   0,
		Minor:   0,
		Patch:   0,
		Channel: "dev",
		Build:   int(cachedBuildTime.Unix()),
	}
	return nil
}
