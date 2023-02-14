// FIXME: \\\\\build integration

package test

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base32"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
	"github.com/superfly/flyctl/internal/cli"
	"github.com/superfly/flyctl/iostreams"
)

func TestAppsV2Example(t *testing.T) {
	var (
		err    error
		result *flyCmdTest

		orgSlug       = "tvd-preflight"        // FIXME: flag / env
		primaryRegion = "sea"                  // FIXME: flag / env
		otherRegions  = []string{"ams", "syd"} // FIXME: flag / env
		// allRegions     = append([]string{primaryRegion}, otherRegions...)
		appName = randomAppName(t)
	)

	result = fly(t, "orgs list --json").Run(t).AssertSuccessfulExit(t)
	var orgMap map[string]string
	err = json.Unmarshal(result.stdOut.Bytes(), &orgMap)
	if err != nil {
		t.Fatalf("failed to parse json: %v [output]: %s\n", err, result.stdOut.String())
	}
	if _, present := orgMap[orgSlug]; !present {
		t.Fatalf("could not find org with name '%s' in `%s` output: %s", orgSlug, result.cmdStr, result.stdOut.String())
	}

	t.Cleanup(func() {
		fly(t, "apps destroy --yes %s", appName).Run(t).AssertSuccessfulExit(t)
	})

	result = fly(t, "launch --org %s --name %s --region %s --image nginx --force-machines --internal-port 80 --now --auto-confirm", orgSlug, appName, primaryRegion).Run(t)
	result.AssertSuccessfulExit(t)
	assert.Contains(t, result.stdOut.String(), "Using image nginx")
	assert.Contains(t, result.stdOut.String(), fmt.Sprintf("Created app %s in organization %s", appName, orgSlug))
	assert.Contains(t, result.stdOut.String(), "Wrote config file fly.toml")
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

type flyCmdTest struct {
	hasRun                bool
	argsStr               string
	args                  []string
	cmdStr                string
	tempDir               string
	testIoStreams         *iostreams.IOStreams
	stdIn, stdOut, stdErr *bytes.Buffer
	exitCode              int
}

func fly(t *testing.T, flyctlCmd string, vals ...interface{}) *flyCmdTest {
	t.Helper()
	argsStr := fmt.Sprintf(flyctlCmd, vals...)
	args, err := shlex.Split(argsStr)
	if err != nil {
		t.Fatalf("failed to parse argStr: %s error: %v", argsStr, err)
	}
	tempDir := t.TempDir()
	t.Setenv("FLY_ACCESS_TOKEN", os.Getenv("FLY_PREFLIGHT_TEST_ACCESS_TOKEN"))
	// t.Setenv("LOG_LEVEL", "debug")
	t.Setenv("HOME", tempDir)
	assert.Nil(t, os.Chdir(tempDir))
	testIoStreams, stdIn, stdOut, stdErr := iostreams.Test()
	return &flyCmdTest{
		argsStr:       argsStr,
		args:          args,
		cmdStr:        fmt.Sprintf("fly %s", argsStr),
		tempDir:       tempDir,
		testIoStreams: testIoStreams,
		stdIn:         stdIn,
		stdOut:        stdOut,
		stdErr:        stdErr,
	}
}

func (f *flyCmdTest) AgentLogs(t *testing.T) string {
	t.Helper()
	logDir := filepath.Join(f.tempDir, ".fly/agent-logs")
	agentLogFiles, err := ioutil.ReadDir(logDir)
	if err != nil {
		t.Fatalf("failed to find agent log files in %s: %v", logDir, err)
	}
	for _, logFile := range agentLogFiles {
		if !logFile.IsDir() {
			filePath := filepath.Join(logDir, logFile.Name())
			if logFile.Size() > 0 {
				content, err := ioutil.ReadFile(filePath)
				if err != nil {
					t.Fatalf("failed to read agent log file at %s: %v", filePath, err)
				}
				return string(content)
			}
		}
	}
	return ""
}

func (f *flyCmdTest) Run(t *testing.T) *flyCmdTest {
	t.Helper()
	f.exitCode = cli.Run(context.TODO(), f.testIoStreams, f.args...)
	f.hasRun = true
	return f
}

func (f *flyCmdTest) AssertSuccessfulExit(t *testing.T) *flyCmdTest {
	// t.Helper()
	if !f.hasRun {
		t.Fatal("cannot call AssertSuccessfulExit() before calling Run() :-(")
	}
	if f.exitCode != 0 {
		t.Fatalf("expected successful zero exit code, got %d, for command: %s [stdout]: %s [strderr]: %s", f.exitCode, f.cmdStr, f.stdOut.String(), f.stdErr.String())
	}
	return f
}
