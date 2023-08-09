//go:build integration
// +build integration

package preflight

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/jpillora/backoff"
	"github.com/stretchr/testify/require"
	"github.com/superfly/flyctl/internal/appconfig"
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
	t.Parallel()

	f := testlib.NewTestEnvFromEnv(t)
	err := copyFixtureIntoWorkDir(f.WorkDir(), "deploy-node", []string{})
	require.NoError(t, err)

	flyTomlPath := fmt.Sprintf("%s/fly.toml", f.WorkDir())
	cfg, err := appconfig.LoadConfig(flyTomlPath)
	require.NoError(t, err)

	appName := f.CreateRandomAppMachines()
	require.NotEmpty(t, appName)

	cfg.AppName = appName
	require.NoError(t, cfg.SetMachinesPlatform())
	cfg.Env["TEST_ID"] = f.ID()

	cfg.WriteToFile(flyTomlPath)

	// testlib.CopyDir(f.WorkDir(), "/Users/gwuah/Desktop/work/fly/flyctl/dirss")

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

	var (
		appUrl = fmt.Sprintf("https://%s.fly.dev", appName)
		resp   *http.Response
	)

	lastStatusCode := -1
	attempts = 10
	b := &backoff.Backoff{Factor: 2, Jitter: true, Min: 100 * time.Millisecond, Max: 5 * time.Second}
	for i := 0; i < attempts; i++ {
		resp, err = http.Get(appUrl)
		if err == nil {
			lastStatusCode = resp.StatusCode
		}
		if lastStatusCode == http.StatusOK {
			break
		}

		time.Sleep(b.Duration())
	}

	body, err := ioutil.ReadAll(resp.Body)
	require.NoError(t, err)

	require.Contains(t, string(body), fmt.Sprintf("Hello, World! %s", f.ID()))
}
