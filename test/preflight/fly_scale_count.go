//go:build integration
// +build integration

package preflight

import (
	"testing"

	"github.com/superfly/flyctl/test/preflight/testlib"
)

func TestFlyScaleCount(t *testing.T) {
	t.Parallel()

	f := testlib.NewTestEnvFromEnv(t)
	appName := f.CreateRandomAppName()

	f.WriteFlyToml(`
app = "%s"
primary_region = "%s"

[build]
	image = "nginx"

[mounts]
	source = "data"
	destination = "/data"
	`, appName, f.PrimaryRegion())

	f.Fly("deploy --ha=false")
	f.Fly("scale count -y 2")
	f.Fly("scale count -y 1 --region %s", f.SecondaryRegion())
	f.Fly("scale count -y 0")
	f.Fly("scale count -y 2")
}
