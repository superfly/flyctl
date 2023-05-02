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
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/internal/appconfig"
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

	result = f.Fly(
		"launch --org %s --name %s --region %s --image nginx --force-machines --internal-port 80 --now --auto-confirm --ha=false",
		f.OrgSlug(), appName, f.PrimaryRegion(),
	)
	require.Contains(f, result.StdOut().String(), "Using image nginx")
	require.Contains(f, result.StdOut().String(), fmt.Sprintf("Created app '%s' in organization '%s'", appName, f.OrgSlug()))
	require.Contains(f, result.StdOut().String(), "Wrote config file fly.toml")

	time.Sleep(10 * time.Second)
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

	machList := f.MachinesList(appName)
	require.Equal(t, len(machList), 1, "There should be exactly one machine")
	firstMachine := machList[0]

	// DisableMachineAutostart is deprecated and should be nil always
	require.Nil(t, firstMachine.Config.DisableMachineAutostart)
	require.Equal(t, 1, len(firstMachine.Config.Services))
	require.NotNil(t, firstMachine.Config.Services[0].Autostart)
	require.NotNil(t, firstMachine.Config.Services[0].Autostop)
	require.True(t, *firstMachine.Config.Services[0].Autostart)
	require.True(t, *firstMachine.Config.Services[0].Autostop)

	secondReg := f.PrimaryRegion()
	if len(f.OtherRegions()) > 0 {
		secondReg = f.OtherRegions()[0]
	}
	f.Fly("m clone --region %s %s", secondReg, firstMachine.ID)

	result = f.Fly("status")
	require.Equal(f, 2, strings.Count(result.StdOut().String(), "started"), "expected 2 machines to be started after cloning the original, instead %s showed: %s", result.CmdString(), result.StdOut().String())

	thirdReg := secondReg
	if len(f.OtherRegions()) > 1 {
		thirdReg = f.OtherRegions()[1]
	}
	f.Fly("m clone --region %s %s", thirdReg, firstMachine.ID)

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
		f.Fatalf("failed to write dockerfile at %s error: %v", dockerfilePath, err)
	}

	f.Fly("deploy --detach")
}

func TestAppsV2ConfigChanges(t *testing.T) {
	var (
		err            error
		f              = testlib.NewTestEnvFromEnv(t)
		appName        = f.CreateRandomAppName()
		configFilePath = filepath.Join(f.WorkDir(), appconfig.DefaultConfigFileName)
	)

	f.Fly(
		"launch --org %s --name %s --region %s --image nginx --internal-port 8080 --force-machines --now --env FOO=BAR",
		f.OrgSlug(), appName, f.PrimaryRegion(),
	)

	f.Fly("config save -a %s -y", appName)
	configFileBytes, err := os.ReadFile(configFilePath)
	if err != nil {
		f.Fatalf("error trying to read %s after running fly config save: %v", configFilePath, err)
	}

	newConfigFile := strings.Replace(string(configFileBytes), `FOO = "BAR"`, `BAR = "QUX"`, 1)
	err = os.WriteFile(configFilePath, []byte(newConfigFile), 0666)
	if err != nil {
		f.Fatalf("error trying to write to fly.toml: %s", err)
	}

	f.Fly("deploy --detach")

	result := f.Fly("config show -a %s", appName)
	require.Contains(f, result.StdOut().String(), `"internal_port": 80`)

	f.Fly("config save -a %s -y", appName)
	configFileBytes, err = os.ReadFile(configFilePath)
	if err != nil {
		f.Fatalf("error trying to read %s after running fly config save: %v", configFilePath, err)
	}

	require.Contains(f, string(configFileBytes), `BAR = "QUX"`)
}

func TestAppsV2ConfigSave_ProcessGroups(t *testing.T) {
	var (
		err            error
		f              = testlib.NewTestEnvFromEnv(t)
		appName        = f.CreateRandomAppMachines()
		configFilePath = filepath.Join(f.WorkDir(), appconfig.DefaultConfigFileName)
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
		configFilePath = filepath.Join(f.WorkDir(), appconfig.DefaultConfigFileName)
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
	f := testlib.NewTestEnvFromEnv(t)
	appName := f.CreateRandomAppName()

	f.Fly(
		"pg create --org %s --name %s --region %s --initial-cluster-size 1 --vm-size shared-cpu-1x --volume-size 1",
		f.OrgSlug(), appName, f.PrimaryRegion(),
	)
	f.Fly("status -a %s", appName)
	f.Fly("config save -a %s", appName)
	flyToml := f.UnmarshalFlyToml()
	want := map[string]any{
		"app":            appName,
		"primary_region": f.PrimaryRegion(),
		"env": map[string]any{
			"PRIMARY_REGION": f.PrimaryRegion(),
		},
		"metrics": map[string]any{
			"port": int64(9187),
			"path": "/metrics",
		},
		"mounts": []map[string]any{{
			"source":      "pg_data",
			"destination": "/data",
		}},
		"services": []map[string]any{
			{
				"internal_port": int64(5432),
				"protocol":      "tcp",
				"concurrency": map[string]any{
					"type":       "connections",
					"soft_limit": int64(1000),
					"hard_limit": int64(1000),
				},
				"ports": []map[string]any{
					{"handlers": []any{"pg_tls"}, "port": int64(5432)},
				},
			},
			{
				"internal_port": int64(5433),
				"protocol":      "tcp",
				"concurrency": map[string]any{
					"type":       "connections",
					"soft_limit": int64(1000),
					"hard_limit": int64(1000),
				},
				"ports": []map[string]any{
					{"handlers": []any{"pg_tls"}, "port": int64(5433)},
				},
			},
		},
		"checks": map[string]any{
			"pg": map[string]any{
				"type":     "http",
				"port":     int64(5500),
				"path":     "/flycheck/pg",
				"interval": "15s",
				"timeout":  "10s",
			},
			"role": map[string]any{
				"type":     "http",
				"port":     int64(5500),
				"path":     "/flycheck/role",
				"interval": "15s",
				"timeout":  "10s",
			},
			"vm": map[string]any{
				"type":     "http",
				"port":     int64(5500),
				"path":     "/flycheck/vm",
				"interval": "15s",
				"timeout":  "10s",
			},
		},
	}
	require.Equal(f, want, flyToml)
}

func TestAppsV2_PostgresAutostart(t *testing.T) {
	var (
		err     error
		f       = testlib.NewTestEnvFromEnv(t)
		appName = f.CreateRandomAppName()
	)

	f.Fly("pg create --org %s --name %s --region %s --initial-cluster-size 1 --vm-size shared-cpu-1x --volume-size 1", f.OrgSlug(), appName, f.PrimaryRegion())

	var machList []map[string]any

	result := f.Fly("m list --json -a %s", appName)
	err = json.Unmarshal(result.StdOut().Bytes(), &machList)
	if err != nil {
		f.Fatalf("failed to parse json: %v [output]: %s\n", err, result.StdOut().String())
	}
	require.Equal(t, 1, len(machList), "expected exactly 1 machine after launch")
	firstMachine := machList[0]

	config := firstMachine["config"].(map[string]interface{})
	if autostart_disabled, ok := config["disable_machine_autostart"]; ok {
		require.Equal(t, true, autostart_disabled.(bool), "autostart was enabled")
	} else {
		f.Fatalf("autostart wasn't disabled")
	}

	appName = f.CreateRandomAppName()

	f.Fly("pg create --org %s --name %s --region %s --initial-cluster-size 1 --vm-size shared-cpu-1x --volume-size 1 --autostart", f.OrgSlug(), appName, f.PrimaryRegion())

	result = f.Fly("m list --json -a %s", appName)
	err = json.Unmarshal(result.StdOut().Bytes(), &machList)
	if err != nil {
		f.Fatalf("failed to parse json: %v [output]: %s\n", err, result.StdOut().String())
	}
	require.Equal(t, 1, len(machList), "expected exactly 1 machine after launch")
	firstMachine = machList[0]

	config = firstMachine["config"].(map[string]interface{})

	if autostart_disabled, ok := config["disable_machine_autostart"]; ok {
		require.Equal(t, false, autostart_disabled.(bool), "autostart was enabled")
	}
}

func TestAppsV2_PostgresNoMachines(t *testing.T) {
	var (
		err     error
		f       = testlib.NewTestEnvFromEnv(t)
		appName = f.CreateRandomAppName()
	)

	f.Fly("pg create --org %s --name %s --region %s --initial-cluster-size 1 --vm-size shared-cpu-1x --volume-size 1", f.OrgSlug(), appName, f.PrimaryRegion())

	// Remove the app's only machine, then run `status`
	result := f.Fly("m list --json -a %s", appName)
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

	f.Fly("m destroy %s -a %s --force", firstMachineId, appName)
	result = f.Fly("status -a %s", appName)

	require.Contains(f, result.StdOut().String(), "No machines are available on this app")
}

func TestAppsV2ConfigSave_PostgresHA(t *testing.T) {
	f := testlib.NewTestEnvFromEnv(t)
	appName := f.CreateRandomAppName()

	f.Fly(
		"pg create --org %s --name %s --region %s --initial-cluster-size 3 --vm-size shared-cpu-1x --volume-size 1",
		f.OrgSlug(), appName, f.PrimaryRegion(),
	)
	f.Fly("status -a %s", appName)
	f.Fly("config save -a %s", appName)
	ml := f.MachinesList(appName)
	require.Equal(f, 3, len(ml))
	flyToml := f.UnmarshalFlyToml()
	require.Equal(f, "shared-cpu-1x", ml[0].Config.Guest.ToSize())
	want := map[string]any{
		"app":            appName,
		"primary_region": f.PrimaryRegion(),
		"env": map[string]any{
			"PRIMARY_REGION": f.PrimaryRegion(),
		},
		"metrics": map[string]any{
			"port": int64(9187),
			"path": "/metrics",
		},
		"mounts": []map[string]any{{
			"source":      "pg_data",
			"destination": "/data",
		}},
		"services": []map[string]any{
			{
				"internal_port": int64(5432),
				"protocol":      "tcp",
				"concurrency": map[string]any{
					"type":       "connections",
					"soft_limit": int64(1000),
					"hard_limit": int64(1000),
				},
				"ports": []map[string]any{
					{"handlers": []any{"pg_tls"}, "port": int64(5432)},
				},
			},
			{
				"internal_port": int64(5433),
				"protocol":      "tcp",
				"concurrency": map[string]any{
					"type":       "connections",
					"soft_limit": int64(1000),
					"hard_limit": int64(1000),
				},
				"ports": []map[string]any{
					{"handlers": []any{"pg_tls"}, "port": int64(5433)},
				},
			},
		},
		"checks": map[string]any{
			"pg": map[string]any{
				"type":     "http",
				"port":     int64(5500),
				"path":     "/flycheck/pg",
				"interval": "15s",
				"timeout":  "10s",
			},
			"role": map[string]any{
				"type":     "http",
				"port":     int64(5500),
				"path":     "/flycheck/role",
				"interval": "15s",
				"timeout":  "10s",
			},
			"vm": map[string]any{
				"type":     "http",
				"port":     int64(5500),
				"path":     "/flycheck/vm",
				"interval": "15s",
				"timeout":  "10s",
			},
		},
	}
	require.Equal(f, want, flyToml)
}

func TestAppsV2Config_ParseExperimental(t *testing.T) {
	var (
		err            error
		f              = testlib.NewTestEnvFromEnv(t)
		appName        = f.CreateRandomAppName()
		configFilePath = filepath.Join(f.WorkDir(), appconfig.DefaultConfigFileName)
	)

	config := `
	[experimental]
	  auto_rollback = true
	`

	err = os.WriteFile(configFilePath, []byte(config), 0644)
	if err != nil {
		f.Fatalf("Failed to write config: %s", err)
	}

	result := f.Fly("launch --no-deploy --force-machines --name %s --region ord --copy-config --org %s", appName, f.OrgSlug())
	stdout := result.StdOut().String()
	require.Contains(f, stdout, "Created app")
	require.Contains(f, stdout, "Wrote config file fly.toml")
}

func TestAppsV2Config_ProcessGroups(t *testing.T) {
	var (
		f              = testlib.NewTestEnvFromEnv(t)
		appName        = f.CreateRandomAppMachines()
		configFilePath = filepath.Join(f.WorkDir(), appconfig.DefaultConfigFileName)
		deployOut      *testlib.FlyctlResult
	)

	// High level view:
	//  1. Create an app with no process groups
	//     • expected: one "app" machine
	//  2. Deploy with two process groups ("web", "bar_web")
	//     • expected: one "web" machine and one "bar_web" machine
	//  3. Clone the "bar_web" machine. This is to ensure that all
	//     machines in a group are destroyed. If possible, this machine
	//     is spawned in a different region from the others, as well.
	//     • expected: one "web" machine, two "bar_web" machines
	//  4. Deploy with one process group ("web")
	//     • expected: one "web" machine
	//  3. Set a secret. This checks the 'restartOnly' deploy cycle.
	//     We set update the machine with a custom metadata entry, as well,
	//     to verify that these are preserved across deploys.
	//     • expected: one "web" machine

	deployToml := func(toml string) *testlib.FlyctlResult {
		toml = "app = \"" + appName + "\"\n" + toml
		err := os.WriteFile(configFilePath, []byte(toml), 0666)
		if err != nil {
			f.Fatalf("error trying to write %s: %v", configFilePath, err)
		}
		cmd := f.Fly("deploy --detach --now --image nginx --ha=false")
		cmd.AssertSuccessfulExit()
		return cmd
	}

	expectMachinesInGroups := func(machines []*api.Machine, expected map[string]int) {
		found := map[string]int{}
		for _, m := range machines {
			if m.Config == nil || m.Config.Metadata == nil {
				f.Fatalf("invalid configuration for machine %s, expected config.metadata != nil", m.ID)
			}
			// When apps machines are deployed, blank process groups should be canonicalized to
			// "app". If they are blank or unset, this is an error state.
			group := "[unspecified]"
			if val, ok := m.Config.Metadata[api.MachineConfigMetadataKeyFlyProcessGroup]; ok {
				group = val
			}
			found[group]++
		}
		if !reflect.DeepEqual(expected, found) {
			err := "groups mismatch:\n"
			for _, group := range []struct {
				name   string
				values map[string]int
			}{
				{name: "expected", values: expected},
				{name: "found", values: found},
			} {
				err += " " + group.name + ":\n"
				for groupName, numMachines := range group.values {
					err += fmt.Sprintf("  %s: %d\n", groupName, numMachines)
				}
				err += "\n"
			}
			f.Fatal(err)
		}
	}

	// Step 1: No process groups defined, should make one "app" machine

	deployOut = deployToml(`
[[services]]
  http_checks = []
  internal_port = 8080
  protocol = "tcp"
  script_checks = []
`)
	require.Contains(t, deployOut.StdOut().String(), `create 1 "app" machine`)

	machines := f.MachinesList(appName)

	expectMachinesInGroups(machines, map[string]int{
		"app": 1,
	})

	// Step 2: Process groups "web" and "bar_web" defined.
	//         Should create two new machines for these apps, in the default region,
	//         and destroy the existing machine in the "app" group.

	deployOut = deployToml(`
[processes]
web = "nginx -g 'daemon off;'"
bar_web = "bash -c 'while true; do sleep 10; done'"

[[services]]
  processes = ["web"] # this service only applies to the web process
  http_checks = []
  internal_port = 8080
  protocol = "tcp"
  script_checks = []
`)
	stdout := deployOut.StdOut().String()
	require.Contains(t, stdout, `destroy 1 "app" machine`)
	require.Contains(t, stdout, `create 1 "web" machine`)
	require.Contains(t, stdout, `create 1 "bar_web" machine`)

	machines = f.MachinesList(appName)

	expectMachinesInGroups(machines, map[string]int{
		"web":     1,
		"bar_web": 1,
	})

	webMachId := ""
	barWebMachId := ""
	for _, m := range machines {
		group := m.Config.Metadata[api.MachineConfigMetadataKeyFlyProcessGroup]
		switch group {
		case "web":
			webMachId = m.ID
			break
		case "bar_web":
			barWebMachId = m.ID
			break
		}
	}
	// This should never be empty; we verified it above in expectMachinesInGroups
	// If you're going to be paranoid anywhere, though, it should be in tests.
	require.NotEmpty(t, webMachId, "could not find 'web' machine. this is a bug in the test.")

	// Step 3: Clone "bar_web" to ensure that all machines get destroyed.
	secondaryRegion := f.PrimaryRegion()
	if len(f.OtherRegions()) > 0 {
		secondaryRegion = f.OtherRegions()[0]
	}
	f.Fly("m clone %s --region %s", barWebMachId, secondaryRegion)
	f.Fly("machine update %s -m ABCD=EFGH -y", webMachId).AssertSuccessfulExit()

	// Step 4: Process group "web" defined.
	//         Should destroy the "bar_web" machines, and keep the same "web" machine.

	deployOut = deployToml(`
[processes]
web = "nginx -g 'daemon off;'"

[[services]]
  processes = ["web"] # this service only applies to the web process
  http_checks = []
  internal_port = 8080
  protocol = "tcp"
  script_checks = []
`)
	require.Contains(t, deployOut.StdOut().String(), `destroy 2 "bar_web" machines`)
	machines = f.MachinesList(appName)

	expectMachinesInGroups(machines, map[string]int{
		"web": 1,
	})

	// Step 5: Set secrets, to ensure that machine data is kept during a 'restartOnly' deploy.
	f.Fly("machine update %s -m CUSTOM=META -y", webMachId).AssertSuccessfulExit()
	f.Fly("secrets set 'SOME=MY_SECRET_TEST_STRING' -a %s", appName).AssertSuccessfulExit()

	machines = f.MachinesList(appName)

	expectMachinesInGroups(machines, map[string]int{
		"web": 1,
	})

	// TODO: Is this assumption sound? Are deploys guaranteed to maintain machine IDs?
	idMatchFound := false
	for _, m := range machines {
		if m.ID == webMachId {
			idMatchFound = true
			// Quick check to make sure the rest of the config is there.
			cmd := m.Config.Init.Cmd
			if len(cmd) <= 0 || cmd[0] != "nginx" {
				f.Fatalf(`Expected command "nginx -g 'daemon off;'", got "%s".`, strings.Join(cmd, " "))
			}
			val, ok := m.Config.Metadata["CUSTOM"]
			require.Equal(t, ok, true, "Expected machine to have metadata['CUSTOM']")
			require.Equal(t, val, "META", "Expected metadata['CUSTOM'] == 'META', got '%s'", val)
			val, ok = m.Config.Metadata["ABCD"]
			require.Equal(t, ok, true, "Expected machine to have metadata['ABCD']")
			require.Equal(t, val, "EFGH", "Expected metadata['ABCD'] == 'EFGH', got '%s'", val)
			break
		}
	}
	require.Equal(t, idMatchFound, true, "could not find 'web' machine with matching machine ID")
}

func TestAppsV2MigrateToV2(t *testing.T) {
	var (
		err     error
		f       = testlib.NewTestEnvFromEnv(t)
		appName = f.CreateRandomAppName()
	)
	f.Fly("launch --org %s --name %s --region %s --now --internal-port 80 --force-nomad --image nginx", f.OrgSlug(), appName, f.PrimaryRegion())
	time.Sleep(3 * time.Second)
	f.Fly("migrate-to-v2 --primary-region %s --yes", f.PrimaryRegion())
	result := f.Fly("status --json")

	var statusMap map[string]any
	err = json.Unmarshal(result.StdOut().Bytes(), &statusMap)
	if err != nil {
		f.Fatalf("failed to parse json: %v [output]: %s\n", err, result.StdOut().String())
	}
	platformVersion, _ := statusMap["PlatformVersion"].(string)
	require.Equal(f, "machines", platformVersion)
}

// This test takes forever. I'm sorry.
func TestAppsV2MigrateToV2_Volumes(t *testing.T) {
	var (
		err     error
		f       = testlib.NewTestEnvFromEnv(t)
		appName = f.CreateRandomAppName()
	)
	// No spaces or quotes, this is sent unescaped to bash :x
	successStr := "myvolumehasloaded"

	f.Fly("launch --org %s --name %s --region %s --internal-port 80 --force-nomad --image nginx", f.OrgSlug(), appName, f.PrimaryRegion())
	f.Fly("vol create -y --app %s -s 2 --region %s vol_test", appName, f.PrimaryRegion())
	{
		toml, err := os.ReadFile("fly.toml")
		if err != nil {
			f.Fatalf("failed to read fly.toml: %s\n", err)
		}
		tomlStr := string(toml) + "\n[[mounts]]\n  source = \"vol_test\"\n  destination = \"/vol\"\n"
		if err = os.WriteFile("fly.toml", []byte(tomlStr), 0644); err != nil {
			f.Fatalf("failed to write fly.toml: %s\n", err)
		}
	}

	assertHasFlag := func() {
		output := f.Fly("ssh console -q -C 'cat /vol/flag.txt'")
		output.AssertSuccessfulExit()
		outStr := string(output.StdOut().Bytes())

		require.Contains(t, outStr, successStr)
	}

	f.Fly("deploy --now --force-nomad")
	time.Sleep(2 * time.Second)
	f.Fly("ssh console -C \"bash -c 'echo %s > /vol/flag.txt && sync'\"", successStr)

	assertHasFlag()

	time.Sleep(9 * time.Second)
	f.Fly("migrate-to-v2 --primary-region %s --yes", f.PrimaryRegion())
	result := f.Fly("status --json")

	var statusMap map[string]any
	err = json.Unmarshal(result.StdOut().Bytes(), &statusMap)
	if err != nil {
		f.Fatalf("failed to parse json: %v [output]: %s\n", err, result.StdOut().String())
	}
	platformVersion, _ := statusMap["PlatformVersion"].(string)
	require.Equal(f, "machines", platformVersion)

	assertHasFlag()
}

func TestNoPublicIPDeployMachines(t *testing.T) {
	var (
		result *testlib.FlyctlResult

		f       = testlib.NewTestEnvFromEnv(t)
		appName = f.CreateRandomAppName()
	)

	f.Fly("launch --org %s --name %s --region %s --now --internal-port 80 --force-machines --image nginx --auto-confirm --no-public-ips", f.OrgSlug(), appName, f.PrimaryRegion())
	result = f.Fly("ips list --json")
	// There should be no ips allocated
	require.Equal(f, "[]\n", result.StdOut().String())
}
func TestNoPublicIPDeployNomad(t *testing.T) {
	var (
		result *testlib.FlyctlResult

		f       = testlib.NewTestEnvFromEnv(t)
		appName = f.CreateRandomAppName()
	)

	f.Fly("launch --org %s --name %s --region %s --now --internal-port 80 --force-machines --image nginx --auto-confirm --no-public-ips", f.OrgSlug(), appName, f.PrimaryRegion())
	result = f.Fly("ips list --json")
	// There should be no ips allocated
	require.Equal(f, "[]\n", result.StdOut().String())
}