//go:build integration
// +build integration

package preflight

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/superfly/flyctl/internal/appv2"
	"github.com/superfly/flyctl/test/preflight/testlib"
)

func TestAppsV2Example(t *testing.T) {
	var (
		err    error
		result *testlib.FlyctlResult
		resp   *http.Response

		f       = testlib.NewTestEnvFromEnv(t)
		appName = f.CreateRandomAppName()
		appUrl  = fmt.Sprintf("https://%s.fly.dev", appName)
	)

	result = f.Fly("launch --org %s --name %s --region %s --image nginx --force-machines --internal-port 80 --now --auto-confirm", f.OrgSlug(), appName, f.PrimaryRegion())
	require.Contains(f, result.StdOut().String(), "Using image nginx")
	require.Contains(f, result.StdOut().String(), fmt.Sprintf("Created app %s in organization %s", appName, f.OrgSlug()))
	require.Contains(f, result.StdOut().String(), "Wrote config file fly.toml")

	time.Sleep(5 * time.Second)
	f.Fly("status")

	lastStatusCode := -1
	attempts := 10
	for i := 0; i < attempts; i++ {
		resp, err = http.Get(appUrl)
		if err == nil {
			lastStatusCode = resp.StatusCode
		}
		if lastStatusCode == http.StatusOK {
			break
		} else {
			time.Sleep(1 * time.Second)
		}
	}
	if lastStatusCode == -1 {
		f.Fatalf("error calling GET %s: %v", appUrl, err)
	}
	if lastStatusCode != http.StatusOK {
		f.Fatalf("GET %s never returned 200 OK response after %d tries; last status code was: %d", appUrl, attempts, lastStatusCode)
	}

	result = f.Fly("m list --json")
	var machList []map[string]any
	err = json.Unmarshal(result.StdOut().Bytes(), &machList)
	if err != nil {
		f.Fatalf("failed to parse json: %v [output]: %s\n", err, result.StdOut().String())
	}
	require.Equal(t, 1, len(machList), "expected exactly 1 machine after launch")
	firstMachine := machList[0]
	firstMachineId, ok := firstMachine["id"].(string)
	if !ok {
		f.Fatalf("could find or convert id key to string from %s, stdout: %s firstMachine: %v", result.CmdString(), result.StdOut().String(), firstMachine)
	}

	secondReg := f.PrimaryRegion()
	if len(f.OtherRegions()) > 0 {
		secondReg = f.OtherRegions()[0]
	}
	f.Fly("m clone --region %s %s", secondReg, firstMachineId)

	result = f.Fly("status")
	require.Equal(f, 2, strings.Count(result.StdOut().String(), "started"), "expected 2 machines to be started after cloning the original, instead %s showed: %s", result.CmdString(), result.StdOut().String())

	thirdReg := secondReg
	if len(f.OtherRegions()) > 1 {
		thirdReg = f.OtherRegions()[1]
	}
	f.Fly("m clone --region %s %s", thirdReg, firstMachineId)

	result = f.Fly("status")
	require.Equal(f, 3, strings.Count(result.StdOut().String(), "started"), "expected 3 machines to be started after cloning the original, instead %s showed: %s", result.CmdString(), result.StdOut().String())

	f.Fly("secrets set PREFLIGHT_TESTING_SECRET=foo")
	result = f.Fly("secrets list")
	require.Contains(f, result.StdOut().String(), "PREFLIGHT_TESTING_SECRET")

	f.Fly("apps restart %s", appName)

	dockerfileContent := `FROM nginx:1.23.3

ENV BUILT_BY_DOCKERFILE=true
`
	dockerfilePath := filepath.Join(f.WorkDir(), "Dockerfile")
	err = os.WriteFile(dockerfilePath, []byte(dockerfileContent), 0644)
	if err != nil {
		t.Fatalf("failed to write dockerfile at %s error: %v", dockerfilePath, err)
	}

	f.Fly("deploy")

	// FIXME: test the rest of the example:

	// fly deploy

	// mounts
	// [mounts]
	// destination = "/my/new/director

	// scaling
	// fly machine stop   9080524f610e87
	// fly machine remove 9080524f610e87
	// fly machine remove --force 0e286039f42e86
	// fly machine update --memory 1024 21781973f03e89
	// fly machine update --cpus 2 21781973f03e89

	// processes
	// fly machine update --metadata fly_process_group=app 21781973f03e89
	// fly machine update --metadata fly_process_group=app 9e784925ad9683
	// fly machine update --metadata fly_process_group=worker 148ed21a031189
	// fly deploy
	// [processes]
	//   app = "nginx -g 'daemon off;'"
	//   worker = "tail -F /dev/null" # not a very useful worker!
	// [[services]]
	//   processes = ["app"]
	// fly machine clone --region gru 21781973f03e89
	// fly machine clone --process-group worker 21781973f03e89

	// call with --detach

	// release commands
	// failure mode:
	// fly machine clone --clear-auto-destroy --clear-cmd MACHINE_ID

	// restart app
	// fly apps restart APP_NAME

	// migrate existing app with machines

	// statics
}

func TestAppsV2ConfigSave_ProcessGroups(t *testing.T) {
	var (
		err            error
		f              = testlib.NewTestEnvFromEnv(t)
		appName        = f.CreateRandomAppMachines()
		configFilePath = filepath.Join(f.WorkDir(), appv2.DefaultConfigFileName)
	)
	f.Fly("m run -a %s --env ENV=preflight --  nginx nginx -g 'daemon off;'", appName)
	f.Fly("m run -a %s --env ENV=preflight --  nginx nginx -g 'daemon off;'", appName)
	f.Fly("m run -a %s --env ENV=preflight --  nginx tail -F /dev/null", appName)
	f.Fly("m list -a %s", appName)
	result := f.Fly("config save -a %s", appName)
	configFileBytes, err := os.ReadFile(configFilePath)
	if err != nil {
		f.Fatalf("error trying to read %s after running fly config save: %v", configFilePath, err)
	}
	configFileContent := string(configFileBytes)
	require.Contains(f, configFileContent, "[env]")
	require.Contains(f, configFileContent, `ENV = "preflight"`)
	require.Contains(f, configFileContent, `[processes]`)
	require.Contains(f, configFileContent, `app = "nginx -g 'daemon off;'"`)
	require.Contains(f, result.StdErr().String(), "Found these additional commands on some machines")
	require.Contains(f, result.StdErr().String(), "tail -F /dev/null")
}

func TestAppsV2ConfigSave_OneMachineNoAppConfig(t *testing.T) {
	var (
		err            error
		f              = testlib.NewTestEnvFromEnv(t)
		appName        = f.CreateRandomAppMachines()
		configFilePath = filepath.Join(f.WorkDir(), appv2.DefaultConfigFileName)
	)
	f.Fly("m run -a %s --env ENV=preflight --  nginx tail -F /dev/null", appName)
	if _, err := os.Stat(configFilePath); !errors.Is(err, os.ErrNotExist) {
		f.Fatalf("config file exists at %s :-(", configFilePath)
	}
	f.Fly("status -a %s", appName)
	f.Fly("config save -a %s", appName)
	configFileBytes, err := os.ReadFile(configFilePath)
	if err != nil {
		f.Fatalf("error trying to read %s after running fly config save: %v", configFilePath, err)
	}
	configFileContent := string(configFileBytes)
	require.Contains(f, configFileContent, "[env]")
	require.Contains(f, configFileContent, `ENV = "preflight"`)
	require.Contains(f, configFileContent, `[processes]`)
	require.Contains(f, configFileContent, `app = "tail -F /dev/null"`)
}

func TestAppsV2ConfigSave_PostgresSingleNode(t *testing.T) {
	var (
		err            error
		f              = testlib.NewTestEnvFromEnv(t)
		appName        = f.CreateRandomAppName()
		configFilePath = filepath.Join(f.WorkDir(), appv2.DefaultConfigFileName)
	)
	f.Fly("pg create --org %s --name %s --region %s --initial-cluster-size 1 --vm-size shared-cpu-1x --volume-size 1", f.OrgSlug(), appName, f.PrimaryRegion())
	f.Fly("status -a %s", appName)
	f.Fly("config save -a %s", appName)
	configFileBytes, err := os.ReadFile(configFilePath)
	if err != nil {
		f.Fatalf("error trying to read %s after running fly config save: %v", configFilePath, err)
	}
	configFileContent := string(configFileBytes)
	require.Contains(f, configFileContent, fmt.Sprintf(`primary_region = "%s"`, f.PrimaryRegion()))
	require.Contains(f, configFileContent, `[env]`)
	require.Contains(f, configFileContent, fmt.Sprintf(`PRIMARY_REGION = "%s"`, f.PrimaryRegion()))
	require.Contains(f, configFileContent, `[metrics]
  port = 9187
  path = "/metrics"`)
	require.Contains(f, configFileContent, `[mounts]
  destination = "/data"`)
	require.Contains(f, configFileContent, `[checks]
  [checks.pg]
    port = 5500
    type = "http"
    interval = "15s"
    timeout = "10s"
    path = "/flycheck/pg"
  [checks.role]
    port = 5500
    type = "http"
    interval = "15s"
    timeout = "10s"
    path = "/flycheck/role"
  [checks.vm]
    port = 5500
    type = "http"
    interval = "1m0s"
    timeout = "10s"
    path = "/flycheck/vm"`)
}

func TestAppsV2ConfigSave_PostgresHA(t *testing.T) {
	var (
		err            error
		f              = testlib.NewTestEnvFromEnv(t)
		appName        = f.CreateRandomAppName()
		configFilePath = filepath.Join(f.WorkDir(), appv2.DefaultConfigFileName)
	)
	f.Fly("pg create --org %s --name %s --region %s --initial-cluster-size 3 --vm-size shared-cpu-1x --volume-size 1", f.OrgSlug(), appName, f.PrimaryRegion())
	f.Fly("status -a %s", appName)
	f.Fly("config save -a %s", appName)
	configFileBytes, err := os.ReadFile(configFilePath)
	if err != nil {
		f.Fatalf("error trying to read %s after running fly config save: %v", configFilePath, err)
	}
	configFileContent := string(configFileBytes)
	require.Contains(f, configFileContent, fmt.Sprintf(`primary_region = "%s"`, f.PrimaryRegion()))
	require.Contains(f, configFileContent, fmt.Sprintf(`[env]
  PRIMARY_REGION = "%s"`, f.PrimaryRegion()))
	require.Contains(f, configFileContent, `[metrics]
  port = 9187
  path = "/metrics"`)
	require.Contains(f, configFileContent, `[mounts]
  destination = "/data"`)
	require.Contains(f, configFileContent, `[checks]
  [checks.pg]
    port = 5500
    type = "http"
    interval = "15s"
    timeout = "10s"
    path = "/flycheck/pg"
  [checks.role]
    port = 5500
    type = "http"
    interval = "15s"
    timeout = "10s"
    path = "/flycheck/role"
  [checks.vm]
    port = 5500
    type = "http"
    interval = "1m0s"
    timeout = "10s"
    path = "/flycheck/vm"`)
}
