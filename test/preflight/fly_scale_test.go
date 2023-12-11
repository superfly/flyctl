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

func TestFlyScaleCount(t *testing.T) {
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
	f.Fly("vol extend -s 4 %s", ml[0].Config.Mounts[0].Volume)
	// Confirm the volume is extended.
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		vl := f.VolumeList(appName)
		require.Equal(c, vl[0].SizeGb, 4)
	}, 10*time.Second, 2*time.Second)

	f.Fly("scale count -y 2")
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		ml = f.MachinesList(appName)
		assert.Equal(c, 2, len(ml))
	}, 10*time.Second, 2*time.Second)

	f.Fly("scale count -y 1 --region %s", f.SecondaryRegion())
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		ml = f.MachinesList(appName)
		assert.Equal(c, 3, len(ml))
	}, 10*time.Second, 2*time.Second)

	f.Fly("scale count -y 0")
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		ml = f.MachinesList(appName)
		assert.Equal(c, 0, len(ml))
	}, 10*time.Second, 1*time.Second)

	f.Fly("scale count -y 2")
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		ml = f.MachinesList(appName)
		assert.Equal(c, 2, len(ml))
	}, 10*time.Second, 1*time.Second)

	vl := f.VolumeList(appName)
	for _, v := range vl {
		require.Equal(f, v.SizeGb, 4)
	}
}
