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

func TestFlyVolume_CreateFromDestroyedVolSnapshot(t *testing.T) {
	f := testlib.NewTestEnvFromEnv(t)
	appName := f.CreateRandomAppMachines()
	createRes := f.Fly("vol create -s 1 -a %s -r %s --yes --json test_destroy", appName, f.PrimaryRegion())
	var vol *fly.Volume
	createRes.StdOutJSON(&vol)
	// create and then destroy a machine
	f.Fly("m run --org %s -a %s -r %s -v %s:/data --build-remote-only nginx", f.OrgSlug(), appName, f.PrimaryRegion(), vol.ID)
	machine := f.MachinesList(appName)[0]
	require.Eventually(f, func() bool {
		machines := f.MachinesList(appName)
		return len(machines) == 1 && machines[0].State == "started"
	}, 1*time.Minute, 1*time.Second, "machine %s never started", machine.ID)
	f.Fly("m destroy --force %s", machine.ID)
	require.Eventually(f, func() bool {
		return len(f.MachinesList(appName)) == 0
	}, 1*time.Minute, 1*time.Second, "machine %s never destroyed", machine.ID)
	f.Fly("vol snapshot create --json %s", vol.ID)
	var snapshot *fly.VolumeSnapshot
	require.Eventually(f, func() bool {
		lsRes := f.Fly("vol snapshot ls --json %s", vol.ID)
		var ls []*fly.VolumeSnapshot
		lsRes.StdOutJSON(&ls)
		for _, s := range ls {
			if time.Since(s.CreatedAt) < 1*time.Hour && s.Status == "created" {
				snapshot = s
				return true
			}
		}
		return false
	}, 1*time.Minute, 1*time.Second, "snapshot never made it to created state")
	f.Fly("vol destroy -y %s", vol.ID)
	require.Eventually(f, func() bool {
		lsRes := f.Fly("vol ls -a %s --all --json", appName)
		var ls []*fly.Volume
		lsRes.StdOutJSON(&ls)
		if len(ls) == 1 {
			return ls[0].State == "pending_destroy"
		}
		return false
	}, 1*time.Minute, 1*time.Second, "volume %s never made it to pending_destroy state", vol.ID)
	ls := f.Fly("vol snapshot ls --json %s", vol.ID)
	var snapshots2 []*fly.VolumeSnapshot
	ls.StdOutJSON(&snapshots2)
	require.Len(f, snapshots2, 1)
	require.Equal(f, snapshot.ID, snapshots2[0].ID)
	require.Equal(f, snapshot.Size, snapshots2[0].Size)
	require.Equal(f, snapshot.CreatedAt, snapshots2[0].CreatedAt)
	fromDestroyedRes := f.Fly("vol create -s 1 -a %s -r %s --yes --json --snapshot-id %s test", appName, f.PrimaryRegion(), snapshot.ID)
	var fromDestroyed *fly.Volume
	fromDestroyedRes.StdOutJSON(&fromDestroyed)
	require.Eventually(f, func() bool {
		lsRes := f.Fly("vol ls -a %s --all --json", appName)
		var ls []*fly.Volume
		lsRes.StdOutJSON(&ls)
		for _, s := range ls {
			if s.State == "created" {
				return true
			}
		}
		return false
	}, 1*time.Minute, 1*time.Second, "final volume %s never made it to created state", vol.ID)
}
