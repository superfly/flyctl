// FIXME: \\\\\build integration

package test

import (
	"context"
	"crypto/rand"
	"encoding/base32"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
)

func randomAppName(t *testing.T) string {
	t.Helper()
	b := make([]byte, 4)
	_, err := rand.Read(b)
	if err != nil {
		t.Fatalf("failed to read from random: %v", err)
	}
	randStr := base32.StdEncoding.EncodeToString(b)
	randStr = strings.Replace(randStr, "=", "z", -1)
	dateStr := time.Now().Format("2006-01")
	return fmt.Sprintf("preflight-%s-%s", dateStr, strings.ToLower(randStr))
}

func TestAppsV2Example(t *testing.T) {
	var (
		err     error
		argsStr string
		args    []string
		cmdStr  string
		cmd     *exec.Cmd
		output  []byte

		ctx            = context.TODO()
		tempDir        = t.TempDir()
		flyctlBin      = "/Users/tvd/src/superfly/flyctl/bin/flyctl"  // FIXME: flag
		flyOrg         = "tvd-preflight"                              // FIXME: flag
		flyAccessToken = os.Getenv("FLY_PREFLIGHT_TEST_ACCESS_TOKEN") // FIXME: flag
		primaryRegion  = "sea"                                        // FIXME: flag
		otherRegions   = []string{"ams", "syd"}                       // FIXME: flag
		// allRegions     = append([]string{primaryRegion}, otherRegions...)
		appName = randomAppName(t)
	)

	argsStr = "orgs list --json"
	args, err = shlex.Split(argsStr)
	if err != nil {
		t.Fatalf("failed to parse argStr: %s error: %v", argsStr, err)
	}
	cmdStr = fmt.Sprintf("%s %s", flyctlBin, argsStr)
	cmd = exec.CommandContext(ctx, flyctlBin, args...)
	cmd.Dir = tempDir
	cmd.Env = append(cmd.Env,
		fmt.Sprintf("FLY_ACCESS_TOKEN=%s", flyAccessToken),
		fmt.Sprintf("HOME=%s", tempDir),
		// "LOG_LEVEL=debug",
	)
	output, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("error [command]: %s [error]: %v [output]: %s", cmdStr, err, string(output))
	}
	var orgMap map[string]string
	err = json.Unmarshal(output, &orgMap)
	if err != nil {
		t.Fatalf("failed to parse json: %v [output]: %s\n", err, string(output))
	}
	if _, present := orgMap[flyOrg]; !present {
		t.Fatalf("could not find org with name '%s' in `%s` output: %s", flyOrg, cmdStr, string(output))
	}

	// argsStr = fmt.Sprintf("apps create --org %s %s", flyOrg, appName)
	// args, err = shlex.Split(argsStr)
	// if err != nil {
	// 	t.Fatalf("failed to parse argStr: %s error: %v", argsStr, err)
	// }
	// cmdStr = fmt.Sprintf("%s %s", flyctlBin, argsStr)
	// cmd = exec.CommandContext(ctx, flyctlBin, args...)
	// cmd.Dir = tempDir
	// cmd.Env = append(cmd.Env, fmt.Sprintf("FLY_ACCESS_TOKEN=%s", flyAccessToken), fmt.Sprintf("HOME=%s", tempDir))
	// output, err = cmd.CombinedOutput()
	// if err != nil {
	// 	t.Fatalf("error [command]: %s [error]: %v [output]: %s", cmdStr, err, string(output))
	// }

	t.Cleanup(func() {
		// FIXME: consolidate run command and arg splitting stuff
		argsStr := fmt.Sprintf("apps destroy --yes %s", appName)
		args, err := shlex.Split(argsStr)
		if err != nil {
			t.Fatalf("failed to parse argStr: %s error: %v", argsStr, err)
		}
		cmdStr := fmt.Sprintf("%s %s", flyctlBin, argsStr)
		cmd := exec.CommandContext(ctx, flyctlBin, args...)
		cmd.Dir = tempDir
		cmd.Env = append(cmd.Env, fmt.Sprintf("FLY_ACCESS_TOKEN=%s", flyAccessToken), fmt.Sprintf("HOME=%s", tempDir))
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("error [command]: %s [error]: %v [output]: %s", cmdStr, err, string(output))
		}
	})

	argsStr = fmt.Sprintf("launch --org %s --name %s --region %s --image nginx --force-machines --internal-port 80 --now --auto-confirm", flyOrg, appName, primaryRegion)
	args, err = shlex.Split(argsStr)
	if err != nil {
		t.Fatalf("failed to parse argStr: %s error: %v", argsStr, err)
	}
	cmdStr = fmt.Sprintf("%s %s", flyctlBin, argsStr)
	cmd = exec.CommandContext(ctx, flyctlBin, args...)
	cmd.Dir = tempDir
	cmd.Env = append(cmd.Env, fmt.Sprintf("FLY_ACCESS_TOKEN=%s", flyAccessToken), fmt.Sprintf("HOME=%s", tempDir))
	output, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("error [command]: %s [error]: %v [output]: %s", cmdStr, err, string(output))
	}
	assert.Equal(t, "FIXME: expected", string(output), "output does not match expected")
	// FIXME: set other regions
	for _, r := range otherRegions {
		// FIXME: set other regions...
		t.Fatalf("FIXME: test launch in other regions! %s", r)
	}

	// FIXME: do an interactive version, too

	// fly launch --image nginx --internal-port 80
	// ? Would you like to set up a Postgresql database now? No
	// ? Would you like to set up an Upstash Redis database now? No
	// ? Would you like to deploy now? Yes
	// ? Will you use statics for this app (see https://fly.io/docs/reference/configuration/#the-statics-sections)? No

	// fly deploy

	// mounts
	// [mounts]
	// destination = "/my/new/director

	// scaling
	// fly machine clone 21781973f03e89
	// fly machine clone --region syd 21781973f03e89
	// fly machine clone --region ams 21781973f03e89
	// fly machine stop 9080524f610e87
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

	// fly secrets set DB_PASSWORD=supersecret

	// release commands
	// failure mode:
	// fly machine clone --clear-auto-destroy --clear-cmd MACHINE_ID

	// restart app
	// fly apps restart APP_NAME

	// migrate existing app with machines

	// statics
}
