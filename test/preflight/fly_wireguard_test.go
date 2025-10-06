//go:build integration
// +build integration

package preflight

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/superfly/flyctl/test/testlib"
)

// cleanupDigOutput removes quotes and spaces to join TXT record parts properly
func cleanupDigOutput(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, `"`, ""), " ", "")
}

func TestFlyWireguardCreate(t *testing.T) {
	f := testlib.NewTestEnvFromEnv(t)

	appName := f.CreateRandomAppName()
	f.WriteFile("Dockerfile", `FROM alpine:3.22
RUN apk add --no-cache bind-tools
CMD ["sleep", "infinity"]
`)
	f.Fly("launch --org %s --name %s --region %s --ha=false --now", f.OrgSlug(), appName, f.PrimaryRegion())

	// Generate a valid peer name (letters, numbers, and dashes only)
	peerName := fmt.Sprintf("test-peer-%s", f.ID())
	path := filepath.Join(t.TempDir(), "wg.conf")
	f.Fly("wg create %s %s %s %s", f.OrgSlug(), f.PrimaryRegion(), peerName, path)
	defer f.Fly("wg remove %s %s", f.OrgSlug(), peerName)

	t.Run("Make sure the config file is created", func(t *testing.T) {
		// Verify the generated wg.conf file exists and has content
		data, err := os.ReadFile(path)
		require.NoError(t, err, "WireGuard config file should exist at: %s", path)
		require.NotEmpty(t, data, "WireGuard config file should not be empty")
	})
	t.Run("Check the peer is visible from the organization", func(t *testing.T) {
		machines := f.MachinesList(appName)
		require.NotEmpty(t, machines, "Should have at least one machine")
		machineID := machines[0].ID

		// The backend is eventually consistent. The peer may not be immediately visible.
		assert.EventuallyWithT(t, func(t *assert.CollectT) {
			result := f.Fly("machine exec -a %s %s 'dig +short aaaa @fdaa::3 %s._peer.internal'", appName, machineID, peerName)
			assert.Contains(t, result.StdOutString(), "fdaa:")

			result = f.Fly("machine exec -a %s %s 'dig +noall +answer txt @fdaa::3 _peer.internal'", appName, machineID)
			assert.Contains(t, cleanupDigOutput(result.StdOutString()), peerName)
		}, 10*time.Second, time.Second)
	})
}
