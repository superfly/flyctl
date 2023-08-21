//go:build integration
// +build integration

package preflight

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/superfly/flyctl/test/preflight/testlib"
)

func TestFlyScaleCount(t *testing.T) {
	t.Parallel()

	f := testlib.NewTestEnvFromEnv(t)
	appName := f.CreateRandomAppMachines()

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
	ml := f.MachinesList(appName)
	require.Equal(f, 1, len(ml))

	f.Fly("scale count -y 2")
	ml = f.MachinesList(appName)
	require.Equal(f, 2, len(ml))

	f.Fly("scale count -y 1 --region %s", f.SecondaryRegion())
	ml = f.MachinesList(appName)
	require.Equal(f, 3, len(ml))

	f.Fly("scale count -y 0")
	ml = f.MachinesList(appName)
	require.Equal(f, 0, len(ml))

	f.Fly("scale count -y 2")
	ml = f.MachinesList(appName)
	require.Equal(f, 2, len(ml))
}
