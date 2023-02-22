//go:build integration
// +build integration

package preflight

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestAppsV2Example(t *testing.T) {
	var (
		err    error
		result *flyctlResult
		resp   *http.Response

		appName = randomAppName(t)
		appUrl  = fmt.Sprintf("https://%s.fly.dev", appName)
		env     = newTestEnvFromEnv(t)
	)

	t.Cleanup(func() {
		env.Fly("apps destroy --yes %s", appName).AssertSuccessfulExit()
	})

	result = env.Fly("launch --org %s --name %s --region %s --image nginx --force-machines --internal-port 80 --now --auto-confirm", env.orgSlug, appName, env.primaryRegion)
	result.AssertSuccessfulExit()
	assert.Contains(t, result.stdOut.String(), "Using image nginx")
	assert.Contains(t, result.stdOut.String(), fmt.Sprintf("Created app %s in organization %s", appName, env.orgSlug))
	assert.Contains(t, result.stdOut.String(), "Wrote config file fly.toml")

	env.Fly("status").AssertSuccessfulExit()

	time.Sleep(5 * time.Second)
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
		env.DebugPrintHistory()
		t.Fatalf("error calling GET %s: %v", appUrl, err)
	}
	if lastStatusCode != http.StatusOK {
		t.Fatalf("GET %s never returned 200 OK response after %d tries; last status code was: %d", appUrl, attempts, lastStatusCode)
	}

	result = env.Fly("m list --json")
	result.AssertSuccessfulExit()
	var machList []map[string]any
	err = json.Unmarshal(result.stdOut.Bytes(), &machList)
	if err != nil {
		t.Fatalf("failed to parse json: %v [output]: %s\n", err, result.stdOut.String())
	}
	assert.Equal(t, 1, len(machList), "expected exactly 1 machine after launch")
	firstMachine := machList[0]
	firstMachineId, ok := firstMachine["id"].(string)
	if !ok {
		t.Fatalf("could find or convert id key to string from %s, stdout: %s firstMachine: %v", result.cmdStr, result.stdOut.String(), firstMachine)
	}

	secondReg := env.primaryRegion
	if len(env.otherRegions) > 0 {
		secondReg = env.otherRegions[0]
	}
	result = env.Fly("m clone --region %s %s", secondReg, firstMachineId)
	result.AssertSuccessfulExit()

	result = env.Fly("status")
	result.AssertSuccessfulExit()
	assert.Equal(t, 2, strings.Count(result.stdOut.String(), "started"), "expected 2 machines to be started after cloning the original, instead %s showed: %s", result.cmdStr, result.stdOut.String())

	thirdReg := secondReg
	if len(env.otherRegions) > 1 {
		thirdReg = env.otherRegions[1]
	}
	result = env.Fly("m clone --region %s %s", thirdReg, firstMachineId)
	result.AssertSuccessfulExit()

	result = env.Fly("status")
	result.AssertSuccessfulExit()
	assert.Equal(t, 3, strings.Count(result.stdOut.String(), "started"), "expected 3 machines to be started after cloning the original, instead %s showed: %s", result.cmdStr, result.stdOut.String())

	result = env.Fly("secrets set PREFLIGHT_TESTING_SECRET=foo")
	result.AssertSuccessfulExit()

	result = env.Fly("secrets list")
	result.AssertSuccessfulExit()
	assert.Contains(t, result.stdOut.String(), "PREFLIGHT_TESTING_SECRET")

	result = env.Fly("apps restart %s", appName)
	result.AssertSuccessfulExit()

	dockerfileContent := `FROM nginx:1.23.3

ENV BUILT_BY_DOCKERFILE=true
`
	dockerfilePath := filepath.Join(env.homeDir, "Dockerfile")
	err = os.WriteFile(dockerfilePath, []byte(dockerfileContent), 0644)
	if err != nil {
		t.Fatalf("failed to write dockerfile at %s error: %v", dockerfilePath, err)
	}

	result = env.Fly("deploy")
	result.AssertSuccessfulExit()

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
