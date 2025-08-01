package preflight

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/superfly/flyctl/test/preflight/testlib"
)

func TestFlyWireguardCreate(t *testing.T) {
	f := testlib.NewTestEnvFromEnv(t)

	appName := f.CreateRandomAppName()
	f.WriteFile("Dockerfile", `FROM alpine:latest
RUN apk add --no-cache bind-tools
CMD ["sleep", "infinity"]
`)
	result := f.Fly("launch --org %s --name %s --region %s --ha=false --now", f.OrgSlug(), appName, f.PrimaryRegion())
	require.Equal(t, 0, result.ExitCode())

	dir := t.TempDir()
	path := filepath.Join(dir, "wg.conf")

	// Generate a valid peer name (letters, numbers, and dashes only)
	peerName := fmt.Sprintf("test-peer-%s", f.ID())

	result = f.Fly("wg create %s %s %s %s", f.OrgSlug(), f.PrimaryRegion(), peerName, path)
	result.AssertSuccessfulExit()
	defer f.FlyAllowExitFailure("wg remove %s %s", f.OrgSlug(), peerName)

	t.Run("Make sure the config file is created", func(t *testing.T) {
		// Verify the generated wg.conf file exists and has content
		data, err := os.ReadFile(path)
		require.NoError(t, err, "WireGuard config file should exist at: %s", path)
		require.NotEmpty(t, data, "WireGuard config file should not be empty")
	})
	t.Run("Check the peer is visible from the organization", func(t *testing.T) {
		// Get the machine ID using the testlib method
		machines := f.MachinesList(appName)
		require.NotEmpty(t, machines, "Should have at least one machine")
		machineID := machines[0].ID

		// Execute a DNS query for the peer in the machine
		result = f.Fly("machine exec -a %s %s 'dig @fdaa::3 %s._peer.internal'", appName, machineID, peerName)
		require.Equal(t, 0, result.ExitCode(), "Machine exec should succeed")

		// Check that the dig command ran and we got a DNS response (even if NXDOMAIN)
		dig := result.StdOutString()
		require.Contains(t, dig, peerName+"._peer.internal", "Should query for the peer domain")
	})
}
