//go:build integration
// +build integration

package preflight

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/test/preflight/testlib"
)

func TestFlyDeployBuildpackNodeAppWithRemoteBuilder(t *testing.T) {
	t.Parallel()

	f := testlib.NewTestEnvFromEnv(t)
	err := f.CopyFixtureIntoWorkDir("deploy-node")
	require.NoError(t, err)

	flyTomlPath := fmt.Sprintf("%s/fly.toml", f.WorkDir())
	cfg, err := appconfig.LoadConfig(flyTomlPath)
	require.NoError(t, err)

	appName := f.CreateRandomAppMachines()
	require.NotEmpty(t, appName)

	cfg.AppName = appName
	cfg.Env["TEST_ID"] = f.ID()

	cfg.WriteToFile(flyTomlPath)

	f.Fly("deploy --remote-only --ha=false")

}
