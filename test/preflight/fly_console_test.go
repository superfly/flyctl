//go:build integration
// +build integration

package preflight

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/superfly/flyctl/test/preflight/testlib"
)

func TestFlyConsole(t *testing.T) {
	// t.Parallel()

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
	output := result.StdOut().String()
	require.Contains(f, output, targetOutput)

	// The console machine should have been destroyed.
	ml := f.MachinesList(appName)
	require.Equal(f, 1, len(ml))
}
