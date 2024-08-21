//go:build integration
// +build integration

package preflight

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/superfly/flyctl/test/preflight/testlib"
)

func extractHostPart(addr string) (string, error) {
	parts := strings.Split(addr, ":")
	if len(parts) != 8 {
		return "", fmt.Errorf("%q must have eight :-delimited parts", addr)
	}
	if parts[0] != "fdaa" {
		return "", fmt.Errorf("%q must start from fdaa:", addr)
	}
	return parts[3] + ":" + parts[4], nil
}

func TestFlyScaleTo3(t *testing.T) {
	t.Run("Without Volume", func(t *testing.T) {
		testFlyScaleToN(t, 3, false)
	})
	t.Run("With Volume", func(t *testing.T) {
		testFlyScaleToN(t, 3, true)
	})
}

func testFlyScaleToN(t *testing.T, n int, withVolume bool) {
	f := testlib.NewTestEnvFromEnv(t)
	appName := f.CreateRandomAppMachines()

	config := fmt.Sprintf(`
app = "%s"
primary_region = "%s"

[build]
image = "nginx"
`,
		appName, f.PrimaryRegion())

	if withVolume {
		config += `
[mounts]
source = "data"
destination = "/data"
`
	}

	f.WriteFlyToml(config)

	f.Fly("deploy --ha=false")
	ml := f.MachinesList(appName)
	assertMachineCount(t, f, appName, 1)

	t.Logf("scale up %s to %d machines", appName, n)
	f.Fly("scale count -y %d", n)
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		assertMachineCount(c, f, appName, n)
	}, 1*time.Minute, 1*time.Second)

	hosts := map[string]struct{}{}

	ml = f.MachinesList(appName)
	for _, m := range ml {
		host, err := extractHostPart(m.PrivateIP)
		assert.NoError(t, err)

		hosts[host] = struct{}{}
	}

	// This may not be true if N is 100,
	// but we'd do our best to distribute machines.
	assert.Equalf(
		t, len(ml), len(hosts),
		"%d machines are on %d hosts (%v)", len(ml), len(hosts), hosts,
	)
}

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

	f.Fly("scale count -y 2")
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		ml = f.MachinesList(appName)
		assert.Equal(c, 2, len(ml))
	}, 10*time.Second, 2*time.Second)

	if f.SecondaryRegion() != "" {
		f.Fly("scale count -y 1 --region %s", f.SecondaryRegion())
		require.EventuallyWithT(t, func(c *assert.CollectT) {
			ml = f.MachinesList(appName)
			assert.Equal(c, 3, len(ml))
		}, 10*time.Second, 2*time.Second)
	}

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
