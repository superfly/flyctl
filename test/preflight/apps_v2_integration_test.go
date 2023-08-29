//go:build integration
// +build integration

package preflight

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/test/preflight/testlib"
)

func TestAppsV2Example(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	f := testlib.NewTestEnvFromEnv(t)
	appName := f.CreateRandomAppName()
	appUrl := fmt.Sprintf("https://%s.fly.dev", appName)

	result := f.Fly(
		"launch --org %s --name %s --region %s --image nginx --force-machines --internal-port 80 --now --auto-confirm --ha=false",
		f.OrgSlug(), appName, f.PrimaryRegion(),
	)
	require.Contains(f, result.StdOutString(), "Using image nginx")
	require.Contains(f, result.StdOutString(), fmt.Sprintf("Created app '%s' in organization '%s'", appName, f.OrgSlug()))
	require.Contains(f, result.StdOutString(), "Wrote config file fly.toml")

	require.EventuallyWithT(t, func(c *assert.CollectT) {
		resp, err := http.Get(appUrl)
		require.NoError(c, err)
		require.Equal(c, http.StatusOK, resp.StatusCode)
	}, 20*time.Second, 1*time.Second, "GET %s never returned 200 OK response 20 seconds", appUrl)

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
	require.Equal(f, 2, strings.Count(result.StdOutString(), "started"), "expected 2 machines to be started after cloning the original, instead %s showed: %s", result.CmdString(), result.StdOutString())

	thirdReg := secondReg
	if len(f.OtherRegions()) > 1 {
		thirdReg = f.OtherRegions()[1]
	}
	f.Fly("m clone --region %s %s", thirdReg, firstMachine.ID)

	result = f.Fly("status")
	require.Equal(f, 3, strings.Count(result.StdOutString(), "started"), "expected 3 machines to be started after cloning the original, instead %s showed: %s", result.CmdString(), result.StdOutString())

	f.Fly("secrets set PREFLIGHT_TESTING_SECRET=foo")
	result = f.Fly("secrets list")
	require.Contains(f, result.StdOutString(), "PREFLIGHT_TESTING_SECRET")

	f.Fly("apps restart %s", appName)

	dockerfileContent := `FROM nginx:1.23.3

ENV BUILT_BY_DOCKERFILE=true
`
	dockerfilePath := filepath.Join(f.WorkDir(), "Dockerfile")
	err := os.WriteFile(dockerfilePath, []byte(dockerfileContent), 0644)
	if err != nil {
		f.Fatalf("failed to write dockerfile at %s error: %v", dockerfilePath, err)
	}

	f.Fly("deploy --detach")
}

func TestAppsV2ConfigChanges(t *testing.T) {
	f := testlib.NewTestEnvFromEnv(t)
	appName := f.CreateRandomAppName()
	configFilePath := filepath.Join(f.WorkDir(), appconfig.DefaultConfigFileName)

	f.Fly(
		"launch --org %s --name %s --region %s --image nginx --internal-port 8080 --force-machines --now --env FOO=BAR",
		f.OrgSlug(), appName, f.PrimaryRegion(),
	)

	f.Fly("config save -a %s -y", appName)
	configFileBytes, err := os.ReadFile(configFilePath)
	require.NoError(t, err, "error trying to read %s after running fly config save", configFilePath)

	newConfigFile := strings.Replace(string(configFileBytes), `FOO = "BAR"`, `BAR = "QUX"`, 1)
	err = os.WriteFile(configFilePath, []byte(newConfigFile), 0666)
	require.NoError(t, err)

	f.Fly("deploy --detach")

	result := f.Fly("config show -a %s", appName)
	require.Contains(f, result.StdOutString(), `"internal_port": 80`)

	f.Fly("config save -a %s -y", appName)
	configFileBytes, err = os.ReadFile(configFilePath)
	require.NoError(t, err, "error trying to read %s after running fly config save", configFilePath)
	require.Contains(f, string(configFileBytes), `BAR = "QUX"`)
}

func TestAppsV2ConfigSave_ProcessGroups(t *testing.T) {
	f := testlib.NewTestEnvFromEnv(t)
	appName := f.CreateRandomAppMachines()
	configFilePath := filepath.Join(f.WorkDir(), appconfig.DefaultConfigFileName)

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
	f := testlib.NewTestEnvFromEnv(t)
	appName := f.CreateRandomAppMachines()
	configFilePath := filepath.Join(f.WorkDir(), appconfig.DefaultConfigFileName)

	f.Fly("m run -a %s --env ENV=preflight --  nginx tail -F /dev/null", appName)
	if _, err := os.Stat(configFilePath); !errors.Is(err, os.ErrNotExist) {
		f.Fatalf("config file exists at %s :-(", configFilePath)
	}
	f.Fly("status -a %s", appName)
	f.Fly("config save -a %s", appName)
	configFileBytes, err := os.ReadFile(configFilePath)
	require.NoError(t, err, "error trying to read %s after running fly config save", configFilePath)

	configFileContent := string(configFileBytes)
	require.Contains(f, configFileContent, "[env]")
	require.Contains(f, configFileContent, `ENV = "preflight"`)
	require.Contains(f, configFileContent, `[processes]`)
	require.Contains(f, configFileContent, `app = "tail -F /dev/null"`)
}

func TestAppsV2Config_ParseExperimental(t *testing.T) {
	f := testlib.NewTestEnvFromEnv(t)
	appName := f.CreateRandomAppName()
	configFilePath := filepath.Join(f.WorkDir(), appconfig.DefaultConfigFileName)

	config := `
	[experimental]
	  auto_rollback = true
	`

	err := os.WriteFile(configFilePath, []byte(config), 0644)
	require.NoError(t, err, "error trying to write %s", configFilePath)

	result := f.Fly("launch --no-deploy --force-machines --name %s --region ord --copy-config --org %s", appName, f.OrgSlug())
	require.Contains(f, result.StdOutString(), "Created app")
	require.Contains(f, result.StdOutString(), "Wrote config file fly.toml")
}

func TestAppsV2Config_ProcessGroups(t *testing.T) {
	f := testlib.NewTestEnvFromEnv(t)
	appName := f.CreateRandomAppMachines()
	configFilePath := filepath.Join(f.WorkDir(), appconfig.DefaultConfigFileName)

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
		require.NoError(t, err, "error trying to write %s", configFilePath)
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

	deployOut := deployToml(`
[[services]]
  http_checks = []
  internal_port = 8080
  protocol = "tcp"
  script_checks = []
`)
	require.Contains(t, deployOut.StdOutString(), `create 1 "app" machine`)

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
	require.Contains(t, deployOut.StdOutString(), `destroy 1 "app" machine`)
	require.Contains(t, deployOut.StdOutString(), `create 1 "web" machine`)
	require.Contains(t, deployOut.StdOutString(), `create 1 "bar_web" machine`)

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
	require.Contains(t, deployOut.StdOutString(), `destroy 2 "bar_web" machines`)
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
	f := testlib.NewTestEnvFromEnv(t)
	appName := f.CreateRandomAppName()

	f.Fly("launch --org %s --name %s --region %s --now --internal-port 80 --force-nomad --image nginx", f.OrgSlug(), appName, f.PrimaryRegion())
	time.Sleep(3 * time.Second)
	f.Fly("migrate-to-v2 --primary-region %s --yes", f.PrimaryRegion())
	result := f.Fly("status --json")

	var statusMap map[string]any
	result.StdOutJSON(&statusMap)
	platformVersion, _ := statusMap["PlatformVersion"].(string)
	require.Equal(f, "machines", platformVersion)
}

// This test takes forever. I'm sorry.
func TestAppsV2MigrateToV2_Volumes(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	f := testlib.NewTestEnvFromEnv(t)
	f.Skipf("not reliably working")
	appName := f.CreateRandomAppName()

	f.Fly("apps create %s -o %s --nomad", appName, f.OrgSlug())
	f.WriteFlyToml(`
app = "%s"
primary_region = "%s"

[build]
  image = "nginx"

[mounts]
	source = "vol_test"
	destination = "/vol"
	`, appName, f.PrimaryRegion())

	f.Fly("vol create -y -s 2 --region %s vol_test", f.PrimaryRegion())
	f.Fly("deploy --now --force-nomad")

	// Give
	time.Sleep(2 * time.Second)
	f.Fly("ssh console -C 'dd if=/dev/random of=/vol/flag.txt bs=1M count=10'")
	f.Fly("ssh console -C 'sync -f /vol/'")

	require.EventuallyWithT(t, func(c *assert.CollectT) {
		r := f.FlyAllowExitFailure("ssh console -q -C 'test -r /vol/flag.txt'")
		assert.Equal(c, 0, r.ExitCode(), "expected successful zero exit code, got %d, for command: %s [stdout]: %s [strderr]: %s", r.ExitCode(), r.CmdString(), r.StdOutString(), r.StdErr().String())
	}, 5*time.Second, 1*time.Second)

	f.Fly("migrate-to-v2 --primary-region %s --yes", f.PrimaryRegion())
	result := f.Fly("status --json")

	var statusMap map[string]any
	result.StdOutJSON(&statusMap)
	platformVersion, _ := statusMap["PlatformVersion"].(string)
	require.Equal(f, "machines", platformVersion)

	require.EventuallyWithT(t, func(c *assert.CollectT) {
		r := f.FlyAllowExitFailure("ssh console -q -C 'test -r /vol/flag.txt'")
		assert.Equal(c, 0, r.ExitCode(), "expected successful zero exit code, got %d, for command: %s [stdout]: %s [strderr]: %s", r.ExitCode(), r.CmdString(), r.StdOutString(), r.StdErr().String())
	}, 5*time.Second, 1*time.Second)
}

// this test is really slow :(
func TestAppsV2MigrateToV2_Autoscaling(t *testing.T) {
	f := testlib.NewTestEnvFromEnv(t)
	appName := f.CreateRandomAppName()

	ctx, cancel := context.WithTimeoutCause(context.Background(), 6*time.Minute, errors.New("test timed out"))
	defer cancel()

	f.FlyC(ctx, "launch --org %s --name %s --region %s --now --internal-port 80 --force-nomad --image nginx", f.OrgSlug(), appName, f.PrimaryRegion())
	time.Sleep(3 * time.Second)
	f.FlyC(ctx, "autoscale set min=2 max=3")
	f.FlyC(ctx, "migrate-to-v2 --primary-region %s --yes", f.PrimaryRegion())
	result := f.FlyC(ctx, "status --json")

	var statusMap map[string]any
	result.StdOutJSON(&statusMap)
	platformVersion, _ := statusMap["PlatformVersion"].(string)
	require.Equal(f, "machines", platformVersion)

	// give time for the request to process
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		machines := f.MachinesList(appName)
		assert.Equal(c, 3, len(machines))

		for _, machine := range machines {
			services := machine.Config.Services
			assert.Equal(c, 1, len(services))

			service := services[0]
			assert.Equal(c, *service.MinMachinesRunning, 2)
		}
	}, 5*time.Second, 1*time.Second)

	result = f.Fly("config show -a %s", appName)

	require.Contains(f, result.StdOutString(), `"min_machines_running": 2,`)
	require.Contains(f, result.StdOutString(), `"auto_start_machines": true,`)
	require.Contains(f, result.StdOutString(), `"auto_stop_machines": true,`)
}

func TestNoPublicIPDeployMachines(t *testing.T) {
	f := testlib.NewTestEnvFromEnv(t)
	appName := f.CreateRandomAppName()
	f.Fly("launch --org %s --name %s --region %s --now --internal-port 80 --force-machines --image nginx --auto-confirm --no-public-ips", f.OrgSlug(), appName, f.PrimaryRegion())
	result := f.Fly("ips list --json")
	// There should be no ips allocated
	require.Equal(f, "[]\n", result.StdOutString())
}

func TestLaunchCpusMem(t *testing.T) {
	f := testlib.NewTestEnvFromEnv(t)
	appName := f.CreateRandomAppName()

	f.Fly("launch --org %s --name %s --region %s --now --internal-port 80 --image nginx --auto-confirm --vm-cpus 4 --vm-memory 8192 --vm-cpukind performance", f.OrgSlug(), appName, f.PrimaryRegion())
	machines := f.MachinesList(appName)
	firstMachineGuest := machines[0].Config.Guest

	require.Equal(f, 4, firstMachineGuest.CPUs)
	require.Equal(f, 8192, firstMachineGuest.MemoryMB)
	require.Equal(f, "performance", firstMachineGuest.CPUKind)
}

func TestLaunchDetach(t *testing.T) {
	f := testlib.NewTestEnvFromEnv(t)
	appName := f.CreateRandomAppName()

	res := f.Fly("launch --org %s --name %s --region %s --now --internal-port 80 --image nginx --auto-confirm --detach", f.OrgSlug(), appName, f.PrimaryRegion())
	require.NotContains(f, res.StdOutString(), "success")

	res = f.Fly("apps destroy --yes %s", appName)

	res = f.Fly("launch --org %s --name %s --region %s --now --internal-port 80 --image nginx --auto-confirm --copy-config", f.OrgSlug(), appName, f.PrimaryRegion())
	require.Contains(f, res.StdOutString(), "success")
}

func TestDeployDetach(t *testing.T) {
	f := testlib.NewTestEnvFromEnv(t)
	appName := f.CreateRandomAppName()

	f.Fly("launch --org %s --name %s --region %s --now --internal-port 80 --image nginx --auto-confirm", f.OrgSlug(), appName, f.PrimaryRegion())

	res := f.Fly("deploy --detach")
	require.NotContains(f, res.StdOutString(), "started")

	res = f.Fly("deploy")
	require.Contains(f, res.StdOutString(), "started")
}

func TestDeployDetachBatching(t *testing.T) {
	f := testlib.NewTestEnvFromEnv(t)
	appName := f.CreateRandomAppName()

	f.Fly("launch --org %s --name %s --region %s --now --internal-port 80 --image nginx --auto-confirm", f.OrgSlug(), appName, f.PrimaryRegion())
	f.Fly("scale count 6 --yes")

	res := f.Fly("deploy --detach")
	require.NotContains(f, res.StdOutString(), "started", false)

	res = f.Fly("deploy")
	require.Contains(f, res.StdOutString(), "started", false)
}

func TestErrOutput(t *testing.T) {
	f := testlib.NewTestEnvFromEnv(t)
	appName := f.CreateRandomAppName()

	f.Fly("launch --org %s --name %s --region %s --now --internal-port 80 --image nginx --auto-confirm", f.OrgSlug(), appName, f.PrimaryRegion())
	machList := f.MachinesList(appName)
	firstMachine := machList[0]

	res := f.FlyAllowExitFailure("machine update --vm-cpus 3 %s --yes", firstMachine.ID)
	require.Contains(f, res.StdErrString(), "invalid number of CPUs")

	res = f.FlyAllowExitFailure("machine update --vm-memory 10 %s --yes", firstMachine.ID)
	require.Contains(f, res.StdErrString(), "invalid memory size")

	f.Fly("machine update --vm-cpus 4 %s --vm-memory 2048 --yes", firstMachine.ID)

	res = f.FlyAllowExitFailure("machine update --vm-memory 256 %s --yes", firstMachine.ID)
	require.Contains(f, res.StdErrString(), "memory size for config is too low")

	res = f.FlyAllowExitFailure("machine update --vm-memory 16384 %s --yes", firstMachine.ID)
	require.Contains(f, res.StdErrString(), "memory size for config is too high")

	res = f.FlyAllowExitFailure("machine update -a %s %s -y --wait-timeout 1 --vm-size performance-1x", appName, firstMachine.ID)
	require.Contains(f, res.StdErrString(), "timeout reached waiting for machine's state to change")
}
