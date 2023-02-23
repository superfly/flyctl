//go:build integration
// +build integration

package preflight

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base32"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/google/shlex"
	"github.com/superfly/flyctl/iostreams"
	"github.com/superfly/flyctl/terminal"
	"github.com/tj/assert"
)

func randomAppName(t *testing.T) string {
	return randomName(t, "preflight")
}

func randomName(t *testing.T, prefix string) string {
	b := make([]byte, 4)
	_, err := rand.Read(b)
	if err != nil {
		t.Fatalf("failed to read from random: %v", err)
	}
	if !strings.HasPrefix(prefix, "preflight") {
		prefix = fmt.Sprintf("preflight-%s", prefix)
	}
	randStr := base32.StdEncoding.EncodeToString(b)
	randStr = strings.Replace(randStr, "=", "z", -1)
	dateStr := time.Now().Format("2006-01")
	return fmt.Sprintf("%s-%s-%s", prefix, dateStr, strings.ToLower(randStr))
}

type flyctlTestEnv struct {
	t               *testing.T
	homeDir         string
	workDir         string
	flyctlBin       string
	orgSlug         string
	primaryRegion   string
	otherRegions    []string
	agentCancelFunc context.CancelFunc
	cmdHistory      []*flyctlResult
}

func currentRepoFlyctl() string {
	_, filename, _, _ := runtime.Caller(0)
	flyctlBin := path.Join(path.Dir(filename), "../..", "bin", "flyctl")
	return flyctlBin
}

const defaultRegion = "iad"

func primaryRegionFromEnv() string {
	regions := os.Getenv("FLY_PREFLIGHT_TEST_FLY_REGIONS")
	if regions == "" {
		terminal.Warnf("no region set with FLY_PREFLIGHT_TEST_FLY_REGIONS so using: %s", defaultRegion)
		return defaultRegion
	}
	pieces := strings.SplitN(regions, " ", 2)
	return pieces[0]
}

func otherRegionsFromEnv() []string {
	regions := os.Getenv("FLY_PREFLIGHT_TEST_FLY_REGIONS")
	if regions == "" {
		return nil
	}
	pieces := strings.Split(regions, " ")
	if len(pieces) > 1 {
		return pieces[1:]
	} else {
		return nil
	}
}

func newTestEnvFromEnv(t *testing.T) *flyctlTestEnv {
	tempDir := t.TempDir()
	env := newTestEnvFromConfig(t, testEnvConfig{
		homeDir:       tempDir,
		workDir:       tempDir,
		flyctlBin:     os.Getenv("FLY_PREFLIGHT_TEST_FLYCTL_BINARY_PATH"),
		orgSlug:       os.Getenv("FLY_PREFLIGHT_TEST_FLY_ORG"),
		primaryRegion: primaryRegionFromEnv(),
		otherRegions:  otherRegionsFromEnv(),
		accessToken:   os.Getenv("FLY_PREFLIGHT_TEST_ACCESS_TOKEN"),
		logLevel:      os.Getenv("FLY_PREFLIGHT_TEST_LOG_LEVEL"),
	})
	return env
}

type testEnvConfig struct {
	homeDir       string
	workDir       string
	flyctlBin     string
	orgSlug       string
	primaryRegion string
	otherRegions  []string
	accessToken   string
	logLevel      string
	noAgentStart  bool
}

func newTestEnvFromConfig(t *testing.T, cfg testEnvConfig) *flyctlTestEnv {
	t.Setenv("FLY_ACCESS_TOKEN", cfg.accessToken)
	if cfg.logLevel != "" {
		t.Setenv("LOG_LEVEL", cfg.logLevel)
	}
	t.Setenv("HOME", cfg.homeDir)
	assert.Nil(t, os.Chdir(cfg.workDir))
	primaryReg := cfg.primaryRegion
	if primaryReg == "" {
		primaryReg = defaultRegion
	}
	flyctlBin := cfg.flyctlBin
	if flyctlBin == "" {
		flyctlBin = currentRepoFlyctl()
		if flyctlBin == "" {
			flyctlBin = "fly"
		}
	}
	testEnv := &flyctlTestEnv{
		t:             t,
		flyctlBin:     flyctlBin,
		primaryRegion: primaryReg,
		otherRegions:  cfg.otherRegions,
		orgSlug:       cfg.orgSlug,
		homeDir:       cfg.homeDir,
		workDir:       cfg.workDir,
	}
	testEnv.verifyTestOrgExists()
	if !cfg.noAgentStart {
		testEnv.agentCancelFunc = testEnv.StartAgent()
	}
	testEnv.setCleanup()
	return testEnv
}

type flyctlResult struct {
	t                     *testing.T
	argsStr               string
	args                  []string
	cmdStr                string
	testIoStreams         *iostreams.IOStreams
	stdIn, stdOut, stdErr *bytes.Buffer
	exitCode              int
}

func (r *flyctlResult) AssertSuccessfulExit() {
	r.t.Helper()
	if r.exitCode != 0 {
		r.t.Fatalf("expected successful zero exit code, got %d, for command: %s [stdout]: %s [strderr]: %s", r.exitCode, r.cmdStr, r.stdOut.String(), r.stdErr.String())
	}
}

func (r *flyctlResult) DebugPrintOutput() {
	r.t.Helper()
	fmt.Printf("DBGCMD: %s\nOUTPUT:\n%s\n", r.cmdStr, r.stdOut.String())
}

func (f *flyctlTestEnv) Fly(flyctlCmd string, vals ...interface{}) *flyctlResult {
	return f.FlyContext(context.TODO(), flyctlCmd, vals...)
}

func (f *flyctlTestEnv) FlyContext(ctx context.Context, flyctlCmd string, vals ...interface{}) *flyctlResult {
	argsStr := fmt.Sprintf(flyctlCmd, vals...)
	args, err := shlex.Split(argsStr)
	if err != nil {
		f.t.Fatalf("failed to parse argStr: %s error: %v", argsStr, err)
	}
	testIostreams, stdIn, stdOut, stdErr := iostreams.Test()
	res := &flyctlResult{
		t:             f.t,
		argsStr:       argsStr,
		args:          args,
		cmdStr:        fmt.Sprintf("%s %s", f.flyctlBin, argsStr),
		testIoStreams: testIostreams,
		stdIn:         stdIn,
		stdOut:        stdOut,
		stdErr:        stdErr,
	}
	cmd := exec.CommandContext(ctx, f.flyctlBin, res.args...)
	cmd.Stdin = testIostreams.In
	cmd.Stdout = testIostreams.Out
	cmd.Stderr = testIostreams.ErrOut
	err = cmd.Start()
	if err != nil {
		f.t.Fatalf("failed to start command: %s [error]: %s", res.cmdStr, err)
	}
	err = cmd.Wait()
	if err == nil {
		res.exitCode = 0
	} else if exitErr, ok := err.(*exec.ExitError); ok {
		res.exitCode = exitErr.ExitCode()
	} else {
		f.t.Fatalf("unexpected error waiting on command: %s [error]: %v", res.cmdStr, err)
	}
	f.cmdHistory = append(f.cmdHistory, res)
	return res
}

func (f *flyctlTestEnv) StartAgent() context.CancelFunc {
	// FIXME: can we stop any existing agents?
	ctx, cancelFunc := context.WithCancel(context.TODO())
	go func() {
		_ = f.FlyContext(ctx, "agent run")
	}()
	return cancelFunc
}

func (f *flyctlTestEnv) DebugPrintHistory() {
	f.t.Helper()
	for i, r := range f.cmdHistory {
		fmt.Printf("%3d:\n", i+1)
		r.DebugPrintOutput()
	}
}

func (f *flyctlTestEnv) setCleanup() {
	f.t.Cleanup(func() {
		f.agentCancelFunc()
		f.Fly("agent stop")
	})
}

func (f *flyctlTestEnv) verifyTestOrgExists() {
	result := f.Fly("orgs list --json")
	result.AssertSuccessfulExit()
	var orgMap map[string]string
	err := json.Unmarshal(result.stdOut.Bytes(), &orgMap)
	if err != nil {
		f.t.Fatalf("failed to parse json: %v [output]: %s\n", err, result.stdOut.String())
	}
	if _, present := orgMap[f.orgSlug]; !present {
		f.t.Fatalf("could not find org with name '%s' in `%s` output: %s", f.orgSlug, result.cmdStr, result.stdOut.String())
	}
}

func (f *flyctlTestEnv) CreateRandomApp() string {
	appName := randomAppName(f.t)
	f.Fly("apps create %s", appName).AssertSuccessfulExit()
	f.t.Cleanup(func() {
		f.Fly("apps destroy --yes %s", appName).AssertSuccessfulExit()
	})
	return appName
}
