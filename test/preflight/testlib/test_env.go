//go:build integration
// +build integration

package testlib

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"testing"

	"github.com/google/shlex"
	"github.com/stretchr/testify/require"
	"github.com/superfly/flyctl/iostreams"
)

type FlyctlTestEnv struct {
	t             testing.TB
	homeDir       string
	workDir       string
	flyctlBin     string
	orgSlug       string
	primaryRegion string
	otherRegions  []string
	cmdHistory    []*FlyctlResult
}

func (f *FlyctlTestEnv) OrgSlug() string {
	return f.orgSlug
}

func (f *FlyctlTestEnv) WorkDir() string {
	return f.workDir
}

func (f *FlyctlTestEnv) PrimaryRegion() string {
	return f.primaryRegion
}

func (f *FlyctlTestEnv) OtherRegions() []string {
	return f.otherRegions
}

func NewTestEnvFromEnv(t testing.TB) *FlyctlTestEnv {
	tempDir := socketSafeTempDir(t)
	env := NewTestEnvFromConfig(t, TestEnvConfig{
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

type TestEnvConfig struct {
	homeDir       string
	workDir       string
	flyctlBin     string
	orgSlug       string
	primaryRegion string
	otherRegions  []string
	accessToken   string
	logLevel      string
}

func NewTestEnvFromConfig(t testing.TB, cfg TestEnvConfig) *FlyctlTestEnv {
	flyctlBin := cfg.flyctlBin
	if flyctlBin == "" {
		flyctlBin = currentRepoFlyctl()
		if flyctlBin == "" {
			flyctlBin = "fly"
		}
	}
	tryToStopAgentsInOriginalHomeDir(t, flyctlBin)
	tryToStopAgentsFromPastPreflightTests(t, flyctlBin)
	t.Setenv("FLY_ACCESS_TOKEN", cfg.accessToken)
	if cfg.logLevel != "" {
		t.Setenv("LOG_LEVEL", cfg.logLevel)
	}
	t.Setenv("HOME", cfg.homeDir)
	require.Nil(t, os.Chdir(cfg.workDir))
	primaryReg := cfg.primaryRegion
	if primaryReg == "" {
		primaryReg = defaultRegion
	}
	testEnv := &FlyctlTestEnv{
		t:             t,
		flyctlBin:     flyctlBin,
		primaryRegion: primaryReg,
		otherRegions:  cfg.otherRegions,
		orgSlug:       cfg.orgSlug,
		homeDir:       cfg.homeDir,
		workDir:       cfg.workDir,
	}
	testEnv.verifyTestOrgExists()
	t.Cleanup(func() {
		testEnv.FlyAllowExitFailure("agent stop")
	})
	return testEnv
}

type testingTWrapper interface {
	Cleanup(func())
	Error(args ...any)
	Errorf(format string, args ...any)
	Fail()
	FailNow()
	Failed() bool
	Fatal(args ...any)
	Fatalf(format string, args ...any)
	Helper()
	Log(args ...any)
	Logf(format string, args ...any)
	Name() string
	Setenv(key, value string)
	Skip(args ...any)
	SkipNow()
	Skipf(format string, args ...any)
	Skipped() bool
	TempDir() string
}

func (f *FlyctlTestEnv) Fly(flyctlCmd string, vals ...interface{}) *FlyctlResult {
	return f.FlyContextAndConfig(context.TODO(), FlyCmdConfig{}, flyctlCmd, vals...)
}

func (f *FlyctlTestEnv) FlyAllowExitFailure(flyctlCmd string, vals ...interface{}) *FlyctlResult {
	return f.FlyContextAndConfig(context.TODO(), FlyCmdConfig{NoAssertSuccessfulExit: true}, flyctlCmd, vals...)
}

type FlyCmdConfig struct {
	NoAssertSuccessfulExit bool
}

func (f *FlyctlTestEnv) FlyContextAndConfig(ctx context.Context, cfg FlyCmdConfig, flyctlCmd string, vals ...interface{}) *FlyctlResult {
	argsStr := fmt.Sprintf(flyctlCmd, vals...)
	args, err := shlex.Split(argsStr)
	if err != nil {
		f.t.Fatalf("failed to parse argStr: %s error: %v", argsStr, err)
	}
	testIostreams, stdIn, stdOut, stdErr := iostreams.Test()
	res := &FlyctlResult{
		t:             f,
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
	if !cfg.NoAssertSuccessfulExit {
		res.AssertSuccessfulExit()
	}
	return res
}

func (f *FlyctlTestEnv) DebugPrintHistory() {
	f.t.Helper()
	if f.Failed() {
		return
	}
	for i, r := range f.cmdHistory {
		fmt.Printf("%3d:\n", i+1)
		r.DebugPrintOutput()
	}
}

func (f *FlyctlTestEnv) verifyTestOrgExists() {
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

func (f *FlyctlTestEnv) CreateRandomAppName() string {
	appName := randomName(f, "preflight")
	f.Cleanup(func() {
		f.FlyAllowExitFailure("apps destroy --yes %s", appName)
	})
	return appName
}

func (f *FlyctlTestEnv) CreateRandomAppMachines() string {
	appName := f.CreateRandomAppName()
	f.Fly("apps create %s --org %s --machines", appName, f.orgSlug).AssertSuccessfulExit()
	return appName
}

// implement the testing.TB interface, so we can print history of flyctl command and output when failing
func (f *FlyctlTestEnv) Cleanup(cleanupFunc func()) {
	f.t.Cleanup(cleanupFunc)
}

func (f *FlyctlTestEnv) Error(args ...any) {
	f.DebugPrintHistory()
	f.t.Error(args...)
}

func (f *FlyctlTestEnv) Errorf(format string, args ...any) {
	f.DebugPrintHistory()
	f.t.Errorf(format, args...)
}

func (f *FlyctlTestEnv) Fail() {
	f.DebugPrintHistory()
	f.t.Fail()
}

func (f *FlyctlTestEnv) FailNow() {
	f.DebugPrintHistory()
	f.t.FailNow()
}

func (f *FlyctlTestEnv) Failed() bool {
	return f.t.Failed()
}

func (f *FlyctlTestEnv) Fatal(args ...any) {
	f.DebugPrintHistory()
	f.t.Fatal(args...)
}

func (f *FlyctlTestEnv) Fatalf(format string, args ...any) {
	f.DebugPrintHistory()
	f.t.Fatalf(format, args...)
}

func (f *FlyctlTestEnv) Helper() {
	f.t.Helper()
}

func (f *FlyctlTestEnv) Log(args ...any) {
	f.t.Log(args...)
}

func (f *FlyctlTestEnv) Logf(format string, args ...any) {
	f.t.Logf(format, args...)
}

func (f *FlyctlTestEnv) Name() string {
	return f.t.Name()
}

func (f *FlyctlTestEnv) Setenv(key, value string) {
	f.t.Setenv(key, value)
}

func (f *FlyctlTestEnv) Skip(args ...any) {
	f.t.Skip(args...)
}

func (f *FlyctlTestEnv) SkipNow() {
	f.t.SkipNow()
}

func (f *FlyctlTestEnv) Skipf(format string, args ...any) {
	f.t.Skipf(format, args...)
}

func (f *FlyctlTestEnv) Skipped() bool {
	return f.t.Skipped()
}

func (f *FlyctlTestEnv) TempDir() string {
	return f.t.TempDir()
}
