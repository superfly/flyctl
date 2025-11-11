//go:build integration
// +build integration

package preflight

import (
	"os"
	"path/filepath"
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

	// The image is based on Debian bookworm.
	f.WriteFlyToml(`
app = "%s"
primary_region = "%s"
console_command = "/bin/echo '%s'"

[build]
  image = "nginx:1.29-bookworm"

[processes]
  app = "/bin/sleep inf"
`,
		appName, f.PrimaryRegion(), targetOutput,
	)

	f.Fly("deploy --ha=false")

	t.Run("console_command", func(t *testing.T) {
		result := f.Fly("console")
		output := result.StdOutString()
		require.Contains(f, output, targetOutput)
	})

	t.Run("dockerfile", func(t *testing.T) {
		dockerfile := filepath.Join(t.TempDir(), "dockerfile")
		err := os.WriteFile(dockerfile, []byte(`
FROM alpine:latest
CMD ["/bin/sleep", "inf"]
`), 0644)
		require.NoError(t, err)

		result := f.Fly("console -a %s --dockerfile %s", appName, dockerfile)
		assert.Contains(t, result.StdOutString(), targetOutput, "console_command is still used")

		// Because of the dockerfile, the image here is Alpine.
		result = f.Fly("console -a %s --dockerfile %s --command 'cat /etc/os-release'", appName, dockerfile)
		assert.Contains(t, result.StdOutString(), "ID=alpine")
	})

	// All the tests above make ephemeral machines. They should be gone eventually.
	assert.EventuallyWithT(t, func(t *assert.CollectT) {
		ml := f.MachinesList(appName)
		assert.Equal(t, 1, len(ml))
	}, 10*time.Second, 1*time.Second, "machines are ephemeral and eventually gone")
}
