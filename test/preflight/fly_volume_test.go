//go:build integration
// +build integration

package preflight

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/superfly/flyctl/test/preflight/testlib"
)

func TestFlyVolumeExtend(t *testing.T) {
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

	f.Fly("vol extend -s 2 %s", ml[0].Config.Mounts[0].Volume)
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		vl := f.VolumeList(appName)
		require.Equal(c, vl[0].SizeGb, 2)
	}, 10*time.Second, 2*time.Second)

	f.Fly("vol extend -s +1 %s", ml[0].Config.Mounts[0].Volume)
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		vl := f.VolumeList(appName)
		require.Equal(c, vl[0].SizeGb, 3)
	}, 10*time.Second, 2*time.Second)

	f.Fly("vol extend -s +1gb %s", ml[0].Config.Mounts[0].Volume)
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		vl := f.VolumeList(appName)
		require.Equal(c, vl[0].SizeGb, 4)
	}, 10*time.Second, 2*time.Second)

	f.Fly("vol extend -s 5gb %s", ml[0].Config.Mounts[0].Volume)
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		vl := f.VolumeList(appName)
		require.Equal(c, vl[0].SizeGb, 5)
	}, 10*time.Second, 2*time.Second)
}
