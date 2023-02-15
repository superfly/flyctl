// FIXME: \\\\\build integration

package test

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base32"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
	"github.com/superfly/flyctl/agent"
	"github.com/superfly/flyctl/agent/server"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/internal/buildinfo"
	"github.com/superfly/flyctl/internal/cli"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/state"
	"github.com/superfly/flyctl/iostreams"
	"github.com/superfly/flyctl/logger"
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
	homeDir               string
	workDir               string
	testIoStreams         *iostreams.IOStreams
	stdIn, stdOut, stdErr *bytes.Buffer
	exitCode              int
	agentClient           *agent.Client
}

func fly(t *testing.T, flyctlCmd string, vals ...interface{}) *flyCmdTest {
	t.Helper()
	tempDir := t.TempDir()
	config := flyCmdConfig{
		homeDir:     tempDir,
		workDir:     tempDir,
		apiBaseUrl:  apiBaseUrlFromEnv(),
		accessToken: os.Getenv("FLY_PREFLIGHT_TEST_ACCESS_TOKEN"),
	}
	config.testIoStreams, config.stdIn, config.stdOut, config.stdErr = iostreams.Test()
	return flyCmd(t, config, flyctlCmd, vals...)
}

const preflightDefaultApiBaseUrl = "https://api.fly.io"

func apiBaseUrlFromEnv() string {
	apiBaseUrl := os.Getenv("FLY_PREFLIGHT_TEST_API_BASE_URL")
	if apiBaseUrl == "" {
		return preflightDefaultApiBaseUrl
	}
	return apiBaseUrl
}

type flyCmdConfig struct {
	flyctlBinName         string
	homeDir               string
	workDir               string
	apiBaseUrl            string
	accessToken           string
	logLevel              string
	testIoStreams         *iostreams.IOStreams
	stdIn, stdOut, stdErr *bytes.Buffer
	noAutoStartAgent      bool
}

func flyCmd(t *testing.T, cfg flyCmdConfig, flyctlCmd string, vals ...interface{}) *flyCmdTest {
	ctx := context.TODO()
	argsStr := fmt.Sprintf(flyctlCmd, vals...)
	args, err := shlex.Split(argsStr)
	if err != nil {
		t.Fatalf("failed to parse argStr: %s error: %v", argsStr, err)
	}
	t.Setenv("FLY_ACCESS_TOKEN", cfg.accessToken)
	if cfg.logLevel != "" {
		t.Setenv("LOG_LEVEL", cfg.logLevel)
	}
	t.Setenv("HOME", cfg.homeDir)
	assert.Nil(t, os.Chdir(cfg.workDir))
	flyctlBinName := "fly"
	if cfg.flyctlBinName != "" {
		flyctlBinName = cfg.flyctlBinName
	}
	var agentClient *agent.Client
	if !cfg.noAutoStartAgent {
		cfgFile := config.New()
		// FIXME: figure out how to do the ~/.fly/config.yml stuff.. currently borken
		cfgPath := filepath.Join(state.ConfigDirectory(ctx), config.FileName)
		if err := cfgFile.ApplyFile(cfgPath); err != nil && !errors.Is(err, fs.ErrNotExist) {
			t.Fatalf("error create config directory for agent: %v", err)
		}
		cfgFile.ApplyEnv()
		cfgFile.ApplyFlags(flag.FromContext(ctx))
		agentIoStreams, _, agentStdOut, agentStdErr := iostreams.Test()
		agentApiClient := api.NewClientWithBaseUrl(cfg.accessToken, buildinfo.Name(), buildinfo.Version().String(), logger.FromEnv(agentIoStreams.ErrOut), cfg.apiBaseUrl)
		logger := log.Default()
		logger.SetFlags(log.Ldate | log.Lmicroseconds | log.Lmsgprefix)
		logger.SetPrefix("srv ")
		logger.SetOutput(agentIoStreams.ErrOut)
		opt := server.Options{
			Socket:     filepath.Join(cfg.homeDir, "fly-agent.sock"),
			Logger:     logger,
			Client:     agentApiClient,
			Background: false,
			ConfigFile: filepath.Join(cfgPath),
		}
		err := server.Run(ctx, opt)
		// agentClient, err = agent.Establish(ctx, agentApiClient)
		// var agentErr *agent.StartError
		// if err != nil && errors.As(err, &agentErr) {
		// 	t.Fatalf("error starting agent: %v [description]: %s", agentErr, agentErr.Description())
		// }
		if err != nil {
			t.Fatalf("error starting agent: %v [stdout]: %s [stderr]: %s", err, agentStdOut.String(), agentStdErr.String())
		}
	}
	return &flyCmdTest{
		argsStr:       argsStr,
		args:          args,
		cmdStr:        fmt.Sprintf("%s %s", flyctlBinName, argsStr),
		homeDir:       cfg.homeDir,
		workDir:       cfg.workDir,
		testIoStreams: cfg.testIoStreams,
		stdIn:         cfg.stdIn,
		stdOut:        cfg.stdOut,
		stdErr:        cfg.stdErr,
		agentClient:   agentClient,
	}
}

func (f *flyCmdTest) Run(t *testing.T) *flyCmdTest {
	t.Helper()
	return f.RunContext(t, context.Background())
}

func (f *flyCmdTest) RunContext(t *testing.T, ctx context.Context) *flyCmdTest {
	t.Helper()
	f.exitCode = cli.Run(ctx, f.testIoStreams, f.args...)
	f.hasRun = true
	err := f.agentClient.Kill(ctx)
	if err != nil {
		t.Fatalf("error stopping agent: %v", err)
	}
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
