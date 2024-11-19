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

func assertHostDistribution(t *testing.T, f *testlib.FlyctlTestEnv, appName string, count int) {
	hosts := map[string][]string{}

	machines := f.MachinesList(appName)
	for _, m := range machines {
		host, err := extractHostPart(m.PrivateIP)
		assert.NoError(t, err)

		hosts[host] = append(hosts[host], m.ID)
	}

	assert.GreaterOrEqualf(
		t, len(hosts), count,
		"%d machines are on %d hosts", len(machines), len(hosts),
	)
	for host, machines := range hosts {
		t.Logf("host %s has %v", host, machines)
	}
}

func TestFlyScaleTo3(t *testing.T) {
	t.Run("Without Volume", func(t *testing.T) {
		t.Parallel()

		testFlyScaleToN(t, 3, false)
	})
	t.Run("With Volume", func(t *testing.T) {
		t.Parallel()

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
	assertMachineCount(t, f, appName, 1)

	t.Logf("scale up %s to %d machines", appName, n)
	f.Fly("scale count -y %d", n)
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		assertMachineCount(c, f, appName, n)
	}, 1*time.Minute, 1*time.Second)

	// Ideally n, but right now we can't guarantee that.
	// So at least, we should have 2 machines.
	assertHostDistribution(t, f, appName, 2)
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
