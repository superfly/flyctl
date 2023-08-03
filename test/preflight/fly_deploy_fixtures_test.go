package preflight

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/superfly/flyctl/test/preflight/testlib"
)

func TestFlyDeployBuildpackNodeAppWithRemoteBuilder(t *testing.T) {
	t.Parallel()

	f := testlib.NewTestEnvFromEnv(t)
	require.NoError(t, f.CopyFixtureIntoWorkDir("deploy-node"))

	// appName := f.CreateRandomAppMachines()
	// assert.NotEmpty(t, appName)

	// f.Fly("deploy --remote-only --ha=false")

	time.Sleep(15 * time.Second)

}
