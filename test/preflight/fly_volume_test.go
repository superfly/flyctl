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

	f.Fly("deploy --buildkit --remote-only --ha=false")
	ml := f.MachinesList(appName)
	require.Equal(f, 1, len(ml))

	f.Fly("volume extend %s --size 4 --app %s", ml[0].Config.Mounts[0].Volume, appName)
	require.EventuallyWithT(f, func(c *assert.CollectT) {
		vl := f.VolumeList(appName)
		require.Equal(c, vl[0].SizeGb, 4)
	}, 10*time.Second, 2*time.Second)

	f.Fly("volume extend %s --size +1 --app %s", ml[0].Config.Mounts[0].Volume, appName)
	require.EventuallyWithT(f, func(c *assert.CollectT) {
		vl := f.VolumeList(appName)
		require.Equal(c, vl[0].SizeGb, 5)
	}, 10*time.Second, 2*time.Second)

	f.Fly("volume extend %s --size +1gb --app %s", ml[0].Config.Mounts[0].Volume, appName)
	require.EventuallyWithT(f, func(c *assert.CollectT) {
		vl := f.VolumeList(appName)
		require.Equal(c, vl[0].SizeGb, 6)
	}, 10*time.Second, 2*time.Second)

	f.Fly("volume extend %s --size 7gb --app %s", ml[0].Config.Mounts[0].Volume, appName)
	require.EventuallyWithT(f, func(c *assert.CollectT) {
		vl := f.VolumeList(appName)
		require.Equal(c, vl[0].SizeGb, 7)
	}, 10*time.Second, 2*time.Second)
}

func testVolumeLs(t *testing.T) {
	f := testlib.NewTestEnvFromEnv(t)
	appName := f.CreateRandomAppMachines()

	var kept *fly.Volume
	j := f.Fly("volume create test_keep --size 1 --app %s --region %s --yes --json", appName, f.PrimaryRegion())
	j.StdOutJSON(&kept)

	var destroyed *fly.Volume
	j = f.Fly("volume create test_destroy --size 1 --app %s --region %s --yes --json", appName, f.PrimaryRegion())
	j.StdOutJSON(&destroyed)

	// Now destroy a volume (remembering to specify the app name)
	f.Fly("volume destroy %s --yes --app %s", destroyed.ID, appName)

	// Deleted volumes shouldn't be shown by default
	assert.EventuallyWithT(f, func(t *assert.CollectT) {
		lsRes := f.Fly("volume ls --app %s --json", appName)
		var ls []*fly.Volume
		lsRes.StdOutJSON(&ls)
		assert.Lenf(t, ls, 1, "volume %s is still visible", destroyed.ID)
		assert.Equal(t, kept.ID, ls[0].ID)
	}, 5*time.Minute, 10*time.Second)

	// Deleted volumes should be shown with --all.
	assert.EventuallyWithT(f, func(t *assert.CollectT) {
		lsAllRes := f.Fly("volume ls --all --app %s --json", appName)

		var lsAll []*fly.Volume
		lsAllRes.StdOutJSON(&lsAll)

		assert.Len(t, lsAll, 2)

		var lsAllIds []string
		for _, v := range lsAll {
			lsAllIds = append(lsAllIds, v.ID)
		}
		assert.Contains(t, lsAllIds, kept.ID)
		assert.Contains(t, lsAllIds, destroyed.ID)
	}, 5*time.Minute, 10*time.Second)
}

func testVolumeFork(t *testing.T) {
	f := testlib.NewTestEnvFromEnv(t)
	appName := f.CreateRandomAppMachines()

	var original *fly.Volume
	j := f.Fly("volume create foobar --json --app %s --region %s --yes", appName, f.PrimaryRegion())
	j.StdOutJSON(&original)

	var fork *fly.Volume
	j = f.Fly("volume fork --json --app %s --region %s %s", appName, f.PrimaryRegion(), original.ID)
	j.StdOutJSON(&fork)

	assert.NotEqual(t, original.Zone, fork.Zone, "forked volume should be in a different zone")
}

func testVolumeCreateFromDestroyedVolSnapshot(tt *testing.T) {
	t := testLogger{tt}

	f := testlib.NewTestEnvFromEnv(t)
	appName := f.CreateRandomAppMachines()
	createRes := f.Fly("volume create test_destroy --size 1 --app %s --region %s --yes --json", appName, f.PrimaryRegion())
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
	f.Fly("volume snapshot create %s --app %s --json", vol.ID, appName)
	var snapshot *fly.VolumeSnapshot
	require.Eventually(f, func() bool {
		lsRes := f.Fly("volume snapshot ls %s --json --app %s", vol.ID, appName)
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

	// Now destroy a volume (remembering to specify the app name)
	t.Logf("Destroy volume %s", vol.ID)
	f.Fly("volume destroy %s --yes --app %s", vol.ID, appName)

	require.EventuallyWithT(f, func(t *assert.CollectT) {
		var ls []*fly.Volume

		j := f.Fly("volume ls --app %s --all --json", appName)
		j.StdOutJSON(&ls)
		assert.Len(t, ls, 1)
		assert.Contains(t, []string{"scheduling_destroy", "pending_destroy", "destroying", "destroyed"}, ls[0].State)
	}, 5*time.Minute, 10*time.Second, "volume %s never made it to a destroy state", vol.ID)

	ls := f.Fly("volume snapshot ls %s --json --app %s", vol.ID, appName)
	var snapshots2 []*fly.VolumeSnapshot
	ls.StdOutJSON(&snapshots2)
	require.NotEmpty(f, snapshots2)
	require.Equal(f, snapshot.ID, snapshots2[len(snapshots2)-1].ID)
	require.Equal(f, snapshot.Size, snapshots2[len(snapshots2)-1].Size)
	require.Equal(f, snapshot.CreatedAt, snapshots2[len(snapshots2)-1].CreatedAt)

	t.Logf("Create volume from %s", snapshot.ID)
	f.Fly("volume create test --size 1 --app %s --region %s --yes --json --snapshot-id %s", appName, f.PrimaryRegion(), snapshot.ID)

	assert.EventuallyWithT(f, func(t *assert.CollectT) {
		var ls []*fly.Volume
		j := f.Fly("volume ls --app %s --json", appName)
		j.StdOutJSON(&ls)

		assert.Len(t, ls, 1)
		assert.Equal(t, "created", ls[0].State)
	}, 5*time.Minute, 10*time.Second, "final volume %s never made it to created state", vol.ID)
}
