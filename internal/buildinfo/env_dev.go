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
	cachedVersion = version.New(cachedBuildTime, "dev", int(cachedBuildTime.Unix()))
	return nil
}
