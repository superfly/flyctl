//go:build integration
// +build integration

package preflight

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/superfly/fly-go"
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

	f.Fly("vol extend -s 4 %s", ml[0].Config.Mounts[0].Volume)
	require.EventuallyWithT(f, func(c *assert.CollectT) {
		vl := f.VolumeList(appName)
		require.Equal(c, vl[0].SizeGb, 4)
	}, 10*time.Second, 2*time.Second)

	f.Fly("vol extend -s +1 %s", ml[0].Config.Mounts[0].Volume)
	require.EventuallyWithT(f, func(c *assert.CollectT) {
		vl := f.VolumeList(appName)
		require.Equal(c, vl[0].SizeGb, 5)
	}, 10*time.Second, 2*time.Second)

	f.Fly("vol extend -s +1gb %s", ml[0].Config.Mounts[0].Volume)
	require.EventuallyWithT(f, func(c *assert.CollectT) {
		vl := f.VolumeList(appName)
		require.Equal(c, vl[0].SizeGb, 6)
	}, 10*time.Second, 2*time.Second)

	f.Fly("vol extend -s 7gb %s", ml[0].Config.Mounts[0].Volume)
	require.EventuallyWithT(f, func(c *assert.CollectT) {
		vl := f.VolumeList(appName)
		require.Equal(c, vl[0].SizeGb, 7)
	}, 10*time.Second, 2*time.Second)
}

func TestFlyVolumeLs(t *testing.T) {
	f := testlib.NewTestEnvFromEnv(t)
	appName := f.CreateRandomAppMachines()
	v1Res := f.Fly("vol create -s 1 -a %s -r %s --yes --json test_keep", appName, f.PrimaryRegion())
	var v1 *fly.Volume
	v1Res.StdOutJSON(&v1)
	v2Res := f.Fly("vol create -s 1 -a %s -r %s --yes --json test_destroy", appName, f.PrimaryRegion())
	var v2 *fly.Volume
	v2Res.StdOutJSON(&v2)
	f.Fly("vol destroy -y %s", v2.ID)
	lsRes := f.Fly("vol ls -a %s --json", appName)
	var ls []*fly.Volume
	lsRes.StdOutJSON(&ls)
	require.Len(f, ls, 1)
	require.Equal(f, v1.ID, ls[0].ID)
	lsAllRes := f.Fly("vol ls --all -a %s --json", appName)
	var lsAll []*fly.Volume
	lsAllRes.StdOutJSON(&lsAll)
	require.Len(f, lsAll, 2)
	var lsAllIds []string
	for _, v := range lsAll {
		lsAllIds = append(lsAllIds, v.ID)
	}
	require.Contains(f, lsAllIds, v1.ID)
	require.Contains(f, lsAllIds, v2.ID)
}
