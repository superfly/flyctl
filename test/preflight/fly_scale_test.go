//go:build integration
// +build integration

package preflight

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/superfly/flyctl/test/preflight/testlib"
)

func TestFlyScaleCount(t *testing.T) {
	// t.Parallel()

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

	// Extend the volume because if not found, scaling will default to 1GB.
	f.Fly("vol extend -s 2 %s", ml[0].Config.Mounts[0].Volume)

	f.Fly("scale count -y 2")
	time.Sleep(2 * time.Second)
	ml = f.MachinesList(appName)
	require.Equal(f, 2, len(ml))

	f.Fly("scale count -y 1 --region %s", f.SecondaryRegion())
	time.Sleep(2 * time.Second)
	ml = f.MachinesList(appName)
	require.Equal(f, 3, len(ml))

	f.Fly("scale count -y 0")
	time.Sleep(2 * time.Second)
	ml = f.MachinesList(appName)
	require.Equal(f, 0, len(ml))

	f.Fly("scale count -y 2")
	time.Sleep(2 * time.Second)
	ml = f.MachinesList(appName)
	require.Equal(f, 2, len(ml))

	vl := f.VolumeList(appName)
	for _, v := range vl {
		require.Equal(f, v.SizeGb, 2)
	}
}
