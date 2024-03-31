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

func TestFlyConsole(t *testing.T) {
	f := testlib.NewTestEnvFromEnv(t)
	appName := f.CreateRandomAppMachines()
	targetOutput := "console test in " + appName

	f.WriteFlyToml(`
app = "%s"
console_command = "/bin/echo '%s'"

[build]
  image = "nginx"

[processes]
  app = "/bin/sleep inf"
`,
		appName, targetOutput,
	)

	f.Fly("deploy --ha=false")

	result := f.Fly("console")
	output := result.StdOutString()
	require.Contains(f, output, targetOutput)

	// Give time for the machine to be destroyed.
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		ml := f.MachinesList(appName)
		assert.Equal(c, 1, len(ml))
	}, 10*time.Second, 1*time.Second)
}
