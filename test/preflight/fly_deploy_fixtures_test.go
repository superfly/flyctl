//go:build integration
// +build integration

package preflight

import (
	"fmt"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/superfly/flyctl/test/preflight/testlib"
)

func getRootPath() string {
	_, b, _, _ := runtime.Caller(0)
	return filepath.Dir(b)
}

func copyFixtureIntoWorkDir(workDir, name string, exclusion []string) error {
	src := fmt.Sprintf("%s/fixtures/%s", getRootPath(), name)
	return testlib.CopyDir(src, workDir, exclusion)
}

func TestFlyDeployBuildpackNodeAppWithRemoteBuilder(t *testing.T) {
	f := testlib.NewTestEnvFromEnv(t)
	err := copyFixtureIntoWorkDir(f.WorkDir(), "deploy-node", []string{})
	require.NoError(t, err)

	flyTomlPath := fmt.Sprintf("%s/fly.toml", f.WorkDir())

	appName := f.CreateRandomAppMachines()
	require.NotEmpty(t, appName)

	err = testlib.OverwriteConfig(flyTomlPath, map[string]any{
		"app": appName,
		"env": map[string]string{
			"TEST_ID": f.ID(),
		},
	})
	require.NoError(t, err)

	// _ = testlib.CopyDir(f.WorkDir(), "/Users/gwuah/Desktop/work/fly/flyctl/dirss", []string{})

	f.Fly("deploy --remote-only --ha=false")

	result := f.Fly("dig %s.internal -a %s --short", appName, appName)
	require.NotEmpty(f, result.StdOut().String())
	require.Empty(f, result.StdErr().String())

	f.Fly("deploy --remote-only --strategy immediate --ha=false")

	attempts := 30
	for {
		attempts -= 1
		if attempts <= 0 {
			t.Fatal("subsequent deploy resulted in 6PN address change")
			return
		}
		result2 := f.Fly("dig %s.internal -a %s --short", appName, appName)
		if result2.StdOut().String() == result.StdOut().String() {
			require.NotEmpty(f, result2.StdOut().String())
			require.Empty(f, result2.StdErr().String())
			break
		}
		time.Sleep(2 * time.Second)
	}

	body, err := testlib.RunHealthCheck(fmt.Sprintf("https://%s.fly.dev", appName))
	require.NoError(t, err)

	require.Contains(t, string(body), fmt.Sprintf("Hello, World! %s", f.ID()))
}

func TestFlyDeployBasicNodeWithWGEnabled(t *testing.T) {
	f := testlib.NewTestEnvFromEnv(t)
	err := copyFixtureIntoWorkDir(f.WorkDir(), "deploy-node", []string{})
	require.NoError(t, err)

	flyTomlPath := fmt.Sprintf("%s/fly.toml", f.WorkDir())

	appName := f.CreateRandomAppMachines()
	require.NotEmpty(t, appName)

	err = testlib.OverwriteConfig(flyTomlPath, map[string]any{
		"app": appName,
		"env": map[string]string{
			"TEST_ID": f.ID(),
		},
	})
	require.NoError(t, err)

	f.Fly("wireguard websockets enable")

	f.Fly("deploy --remote-only --ha=false")

	result := f.Fly("dig %s.internal -a %s --short", appName, appName)
	require.NotEmpty(f, result.StdOut().String())
	require.Empty(f, result.StdErr().String())

	f.Fly("wireguard websockets disable")

	body, err := testlib.RunHealthCheck(fmt.Sprintf("https://%s.fly.dev", appName))
	require.NoError(t, err)

	require.Contains(t, string(body), fmt.Sprintf("Hello, World! %s", f.ID()))
}
