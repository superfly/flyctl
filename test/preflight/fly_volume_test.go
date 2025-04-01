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

// testLogger adds timestamps to t.Logf().
// This would be helpful to find slow operations.
type testLogger struct {
	testing.TB
}

func (t testLogger) Logf(format string, args ...interface{}) {
	ts := time.Now().Format("15:04:05 ")
	t.TB.Logf(ts+format, args...)
}

func WithParallel(f func(*testing.T)) func(*testing.T) {
	return func(t *testing.T) {
		t.Parallel()
		f(t)
	}
}

func TestVolume(t *testing.T) {
	t.Run("Extend", WithParallel(testVolumeExtend))
	t.Run("List", WithParallel(testVolumeLs))
	t.Run("CreateFromDestroyedVolSnapshot", WithParallel(testVolumeCreateFromDestroyedVolSnapshot))
	t.Run("Fork", WithParallel(testVolumeFork))
}

func testVolumeExtend(t *testing.T) {
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

func testVolumeLs(t *testing.T) {
	f := testlib.NewTestEnvFromEnv(t)
	appName := f.CreateRandomAppMachines()

	var kept *fly.Volume
	j := f.Fly("vol create -s 1 -a %s -r %s --yes --json test_keep", appName, f.PrimaryRegion())
	j.StdOutJSON(&kept)

	var destroyed *fly.Volume
	j = f.Fly("vol create -s 1 -a %s -r %s --yes --json test_destroy", appName, f.PrimaryRegion())
	j.StdOutJSON(&destroyed)
	f.Fly("vol destroy -y %s", destroyed.ID)

	// Deleted volumes shouldn't be shown.
	assert.EventuallyWithT(f, func(c *assert.CollectT) {
		lsRes := f.Fly("vol ls -a %s --json", appName)
		var ls []*fly.Volume
		lsRes.StdOutJSON(&ls)
		assert.Lenf(f, ls, 1, "volume %s is still visible", destroyed.ID)
		assert.Equal(f, kept.ID, ls[0].ID)
	}, 5*time.Minute, 10*time.Second)

	// Deleted volumes should be shown with --all.
	assert.EventuallyWithT(f, func(c *assert.CollectT) {
		lsAllRes := f.Fly("vol ls --all -a %s --json", appName)

		var lsAll []*fly.Volume
		lsAllRes.StdOutJSON(&lsAll)

		assert.Len(f, lsAll, 2)

		var lsAllIds []string
		for _, v := range lsAll {
			lsAllIds = append(lsAllIds, v.ID)
		}
		assert.Contains(f, lsAllIds, kept.ID)
		assert.Contains(f, lsAllIds, destroyed.ID)
	}, 5*time.Minute, 10*time.Second)
}

func testVolumeFork(t *testing.T) {
	f := testlib.NewTestEnvFromEnv(t)
	appName := f.CreateRandomAppMachines()

	var original *fly.Volume
	j := f.Fly("vol create --json --app %s --region %s --yes foobar", appName, f.PrimaryRegion())
	j.StdOutJSON(&original)

	var fork *fly.Volume
	j = f.Fly("vol fork --json --app %s --region %s %s", appName, f.PrimaryRegion(), original.ID)
	j.StdOutJSON(&fork)

	assert.NotEqual(t, original.Zone, fork.Zone, "forked volume should be in a different zone")
}

func testVolumeCreateFromDestroyedVolSnapshot(tt *testing.T) {
	t := testLogger{tt}

	f := testlib.NewTestEnvFromEnv(t)
	appName := f.CreateRandomAppMachines()
	createRes := f.Fly("vol create -s 1 -a %s -r %s --yes --json test_destroy", appName, f.PrimaryRegion())
	var vol *fly.Volume
	createRes.StdOutJSON(&vol)
	t.Logf("Start a machine under app %s", appName)
	f.Fly("m run --org %s -a %s -r %s -v %s:/data --build-remote-only nginx", f.OrgSlug(), appName, f.PrimaryRegion(), vol.ID)
	machine := f.MachinesList(appName)[0]
	require.Eventually(f, func() bool {
		machines := f.MachinesList(appName)
		return len(machines) == 1 && machines[0].State == "started"
	}, 1*time.Minute, 1*time.Second, "machine %s never started", machine.ID)

	f.Fly("m destroy --force %s", machine.ID)
	require.Eventually(f, func() bool {
		return len(f.MachinesList(appName)) == 0
	}, 5*time.Minute, 10*time.Second, "machine %s never destroyed", machine.ID)

	t.Logf("machine %s is destroyed; Snapshot volume %s", machine.ID, vol.ID)
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

	t.Logf("Destroy volume %s", vol.ID)
	f.Fly("vol destroy -y %s", vol.ID)
	require.EventuallyWithT(f, func(t *assert.CollectT) {
		var ls []*fly.Volume

		j := f.Fly("vol ls -a %s --all --json", appName)
		j.StdOutJSON(&ls)

		assert.Equal(t, "pending_destroy", ls[0].State)
		assert.Len(t, ls, 1)
	}, 5*time.Minute, 10*time.Second, "volume %s never made it to pending_destroy state", vol.ID)

	ls := f.Fly("vol snapshot ls --json %s", vol.ID)
	var snapshots2 []*fly.VolumeSnapshot
	ls.StdOutJSON(&snapshots2)
	require.Len(f, snapshots2, 1)
	require.Equal(f, snapshot.ID, snapshots2[0].ID)
	require.Equal(f, snapshot.Size, snapshots2[0].Size)
	require.Equal(f, snapshot.CreatedAt, snapshots2[0].CreatedAt)

	t.Logf("Create volume from %s", snapshot.ID)
	f.Fly("vol create -s 1 -a %s -r %s --yes --json --snapshot-id %s test", appName, f.PrimaryRegion(), snapshot.ID)

	assert.EventuallyWithT(f, func(t *assert.CollectT) {
		var ls []*fly.Volume
		j := f.Fly("vol ls -a %s --json", appName)
		j.StdOutJSON(&ls)

		assert.Len(t, ls, 1)
		assert.Equal(t, "created", ls[0].State)
	}, 5*time.Minute, 10*time.Second, "final volume %s never made it to created state", vol.ID)
}
