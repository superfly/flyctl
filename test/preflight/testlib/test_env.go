//go:build integration
// +build integration

package testlib

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/shlex"
	"github.com/oklog/ulid/v2"
	"github.com/pelletier/go-toml/v2"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/iostreams"
)

type FlyctlTestEnv struct {
	t                   testing.TB
	homeDir             string
	workDir             string
	originalAccessToken string
	flyctlBin           string
	orgSlug             string
	primaryRegion       string
	otherRegions        []string
	env                 map[string]string
	cmdHistory          []*FlyctlResult
	noHistoryOnFail     bool
	id                  string
	VMSize              string
}

func (f *FlyctlTestEnv) OrgSlug() string {
	return f.orgSlug
}

func (f *FlyctlTestEnv) WorkDir() string {
	return f.workDir
}

func (f *FlyctlTestEnv) ID() string {
	return f.id
}

func (f *FlyctlTestEnv) PrimaryRegion() string {
	return f.primaryRegion
}

func (f *FlyctlTestEnv) SecondaryRegion() string {
	if len(f.otherRegions) == 0 {
		return ""
	}
	return f.otherRegions[0]
}

func (f *FlyctlTestEnv) OtherRegions() []string {
	return f.otherRegions
}

// Great name I know
func NewTestEnvFromEnvWithEnv(t testing.TB, envVariables map[string]string) *FlyctlTestEnv {
	tempDir := socketSafeTempDir(t)
	_, noHistoryOnFail := os.LookupEnv("FLY_PREFLIGHT_TEST_NO_PRINT_HISTORY_ON_FAIL")
	testEnv := NewTestEnvFromConfig(t, TestEnvConfig{
		homeDir:         tempDir,
		workDir:         tempDir,
		flyctlBin:       os.Getenv("FLY_PREFLIGHT_TEST_FLYCTL_BINARY_PATH"),
		orgSlug:         os.Getenv("FLY_PREFLIGHT_TEST_FLY_ORG"),
		primaryRegion:   primaryRegionFromEnv(),
		otherRegions:    otherRegionsFromEnv(),
		accessToken:     os.Getenv("FLY_PREFLIGHT_TEST_ACCESS_TOKEN"),
		logLevel:        os.Getenv("FLY_PREFLIGHT_TEST_LOG_LEVEL"),
		noHistoryOnFail: noHistoryOnFail,
		envVariables:    envVariables,
	})
	return testEnv
}

func NewTestEnvFromEnv(t testing.TB) *FlyctlTestEnv {
	tempDir := socketSafeTempDir(t)
	_, noHistoryOnFail := os.LookupEnv("FLY_PREFLIGHT_TEST_NO_PRINT_HISTORY_ON_FAIL")
	env := NewTestEnvFromConfig(t, TestEnvConfig{
		homeDir:         tempDir,
		workDir:         tempDir,
		flyctlBin:       os.Getenv("FLY_PREFLIGHT_TEST_FLYCTL_BINARY_PATH"),
		orgSlug:         os.Getenv("FLY_PREFLIGHT_TEST_FLY_ORG"),
		primaryRegion:   primaryRegionFromEnv(),
		otherRegions:    otherRegionsFromEnv(),
		accessToken:     os.Getenv("FLY_PREFLIGHT_TEST_ACCESS_TOKEN"),
		logLevel:        os.Getenv("FLY_PREFLIGHT_TEST_LOG_LEVEL"),
		noHistoryOnFail: noHistoryOnFail,
		envVariables:    make(map[string]string),
	})

	// annotate github actions output with cli errors
	env.Setenv("FLY_GHA_ERROR_ANNOTATION", "1")
	env.Setenv("GITHUB_ACTIONS", os.Getenv("GITHUB_ACTIONS"))

	t.Logf("workdir %s", env.workDir)
	return env
}

type TestEnvConfig struct {
	homeDir         string
	workDir         string
	flyctlBin       string
	orgSlug         string
	primaryRegion   string
	otherRegions    []string
	accessToken     string
	logLevel        string
	noHistoryOnFail bool
	envVariables    map[string]string
}

func (t *TestEnvConfig) Setenv(name string, value string) {
	t.envVariables[name] = value
}

func NewTestEnvFromConfig(t testing.TB, cfg TestEnvConfig) *FlyctlTestEnv {
	flyctlBin := cfg.flyctlBin
	if flyctlBin == "" {
		flyctlBin = currentRepoFlyctl()
		if flyctlBin == "" {
			flyctlBin = "fly"
		}
	}
	tryToStopAgentsInOriginalHomeDir(flyctlBin)
	// tryToStopAgentsFromPastPreflightTests(t, flyctlBin)
	cfg.Setenv("FLY_ACCESS_TOKEN", cfg.accessToken)
	if cfg.logLevel != "" {
		cfg.Setenv("LOG_LEVEL", cfg.logLevel)
	}
	cfg.Setenv("HOME", cfg.homeDir)
	primaryReg := cfg.primaryRegion
	if primaryReg == "" {
		primaryReg = defaultRegion
	}
	testEnv := &FlyctlTestEnv{
		id:                  ulid.Make().String(),
		t:                   t,
		flyctlBin:           flyctlBin,
		primaryRegion:       primaryReg,
		otherRegions:        cfg.otherRegions,
		orgSlug:             cfg.orgSlug,
		homeDir:             cfg.homeDir,
		workDir:             cfg.workDir,
		originalAccessToken: cfg.accessToken,
		noHistoryOnFail:     cfg.noHistoryOnFail,
		env:                 cfg.envVariables,
		VMSize:              os.Getenv("FLY_PREFLIGHT_TEST_VM_SIZE"),
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

// Fly runs a flyctl the result
func (f *FlyctlTestEnv) Fly(flyctlCmd string, vals ...interface{}) *FlyctlResult {
	if f.VMSize != "" {
		if strings.HasPrefix(flyctlCmd, "machine run ") || strings.HasPrefix(flyctlCmd, "launch ") {
			flyctlCmd += fmt.Sprintf(" --vm-size %s ", f.VMSize)
		}
	}

	return f.FlyContextAndConfig(context.TODO(), FlyCmdConfig{}, flyctlCmd, vals...)
}

// FlyAllowExitFailure runs a flyctl command and returns the result, but does not fail the test if the command exits with a non-zero status
func (f *FlyctlTestEnv) FlyAllowExitFailure(flyctlCmd string, vals ...interface{}) *FlyctlResult {
	return f.FlyContextAndConfig(context.TODO(), FlyCmdConfig{NoAssertSuccessfulExit: true}, flyctlCmd, vals...)
}

// FlyC runs a flyctl command with a context and returns the result
func (f *FlyctlTestEnv) FlyC(ctx context.Context, flyctlCmd string, vals ...interface{}) *FlyctlResult {
	return f.FlyContextAndConfig(ctx, FlyCmdConfig{}, flyctlCmd, vals...)
}

// func (f *FlyctlTestEnv) FlyAllowExitFailure(ctx context.Context, flyctlCmd string, vals ...interface{}) *FlyctlResult {
// 	return f.FlyContextAndConfig(ctx, FlyCmdConfig{NoAssertSuccessfulExit: true}, flyctlCmd, vals...)
// }

type FlyCmdConfig struct {
	NoAssertSuccessfulExit bool
}

func (f *FlyctlTestEnv) FlyContextAndConfig(ctx context.Context, cfg FlyCmdConfig, flyctlCmd string, vals ...interface{}) *FlyctlResult {
	argsStr := fmt.Sprintf(flyctlCmd, vals...)
	args, err := shlex.Split(argsStr)
	if err != nil {
		f.Fatalf("failed to parse argStr: %s error: %v", argsStr, err)
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

	var env []string

	for key, val := range f.env {
		env = append(env, fmt.Sprintf("%s=%s", key, val))
	}

	cmd.Dir = f.workDir
	cmd.Env = env
	cmd.Stdin = testIostreams.In
	cmd.Stdout = testIostreams.Out
	cmd.Stderr = testIostreams.ErrOut
	err = cmd.Start()
	if err != nil {
		f.Fatalf("failed to start command: %s [error]: %s", res.cmdStr, err)
	}
	err = cmd.Wait()
	if err == nil {
		res.exitCode = 0
	} else if exitErr, ok := err.(*exec.ExitError); ok {
		res.exitCode = exitErr.ExitCode()
	} else {
		f.Fatalf("unexpected error waiting on command: %s [error]: %v", res.cmdStr, err)
	}
	f.cmdHistory = append(f.cmdHistory, res)
	if !cfg.NoAssertSuccessfulExit {
		res.AssertSuccessfulExit()
	}
	return res
}

func (f *FlyctlTestEnv) OverrideAuthAccessToken(newToken string) {
	f.Setenv("FLY_ACCESS_TOKEN", newToken)
}

func (f *FlyctlTestEnv) ResetAuthAccessToken() {
	f.Setenv("FLY_ACCESS_TOKEN", f.originalAccessToken)
}

func (f *FlyctlTestEnv) DebugPrintHistory() {
	f.t.Helper()
	if f.Failed() || f.noHistoryOnFail {
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
	result.StdOutJSON(&orgMap)
	if _, present := orgMap[f.orgSlug]; !present {
		f.Fatalf("could not find org with name '%s' in `%s` output: %s", f.orgSlug, result.cmdStr, result.stdOut.String())
	}
}

func (f *FlyctlTestEnv) CreateRandomAppName() string {
	prefix := os.Getenv("FLY_PREFLIGHT_TEST_APP_PREFIX")
	if prefix == "" {
		prefix = "preflight"
	}

	appName := randomName(f, prefix)
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

func (f *FlyctlTestEnv) MachinesList(appName string) []*fly.Machine {
	time.Sleep(800 * time.Millisecond) // fly m list is eventually consistent, yay!
	cmdResult := f.Fly("machines list --app %s --json", appName)
	cmdResult.AssertSuccessfulExit()
	var machList []*fly.Machine
	cmdResult.StdOutJSON(&machList)
	return machList
}

func (f *FlyctlTestEnv) VolumeList(appName string) []*fly.Volume {
	cmdResult := f.Fly("volume list --app %s --json", appName)
	var list []*fly.Volume
	cmdResult.StdOutJSON(&list)
	return list
}

func (f *FlyctlTestEnv) WriteFile(path string, format string, vals ...any) {
	fn := filepath.Join(f.WorkDir(), path)
	content := fmt.Sprintf(format, vals...)
	if err := os.WriteFile(fn, []byte(content), 0666); err != nil {
		f.Fatalf("error writing to %s: %v", path, err)
	}
}

func (f *FlyctlTestEnv) ReadFile(path string) string {
	fn := filepath.Join(f.WorkDir(), path)
	content, err := os.ReadFile(fn)
	if err != nil {
		f.Fatalf("error reading from %s: %v", path, err)
	}
	return string(content)
}

// WriteFlyToml writes a fly.toml file with the given format and values
func (f *FlyctlTestEnv) WriteFlyToml(format string, vals ...any) {
	f.WriteFile("fly.toml", format, vals...)
}

func (f *FlyctlTestEnv) UnmarshalFlyToml() (res map[string]any) {
	data := f.ReadFile("fly.toml")
	if err := toml.Unmarshal([]byte(data), &res); err != nil {
		f.Fatalf("error parsing fly.toml: %v", err)
	}
	return
}

// implement the testing.TB interface, so we can print history of flyctl command and output when failing
func (f *FlyctlTestEnv) Cleanup(cleanupFunc func()) {
	f.t.Cleanup(cleanupFunc)
}

func (f *FlyctlTestEnv) Error(args ...any) {
	f.t.Helper()
	f.DebugPrintHistory()
	f.t.Error(args...)
}

func (f *FlyctlTestEnv) Errorf(format string, args ...any) {
	f.t.Helper()
	f.DebugPrintHistory()
	f.t.Errorf(format, args...)
}

func (f *FlyctlTestEnv) Fail() {
	f.t.Helper()
	f.DebugPrintHistory()
	f.t.Fail()
}

func (f *FlyctlTestEnv) FailNow() {
	f.t.Helper()
	f.DebugPrintHistory()
	f.t.FailNow()
}

func (f *FlyctlTestEnv) Failed() bool {
	f.t.Helper()
	return f.t.Failed()
}

func (f *FlyctlTestEnv) Fatal(args ...any) {
	f.t.Helper()
	f.DebugPrintHistory()
	f.t.Fatal(args...)
}

func (f *FlyctlTestEnv) Fatalf(format string, args ...any) {
	f.t.Helper()
	f.DebugPrintHistory()
	f.t.Fatalf(format, args...)
}

func (f *FlyctlTestEnv) Helper() {
	f.t.Helper()
}

func (f *FlyctlTestEnv) Log(args ...any) {
	f.t.Helper()
	f.t.Log(args...)
}

func (f *FlyctlTestEnv) Logf(format string, args ...any) {
	f.t.Helper()
	f.t.Logf(format, args...)
}

func (f *FlyctlTestEnv) Name() string {
	return f.t.Name()
}

func (f *FlyctlTestEnv) Setenv(key, value string) {
	f.env[key] = value
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

func (f *FlyctlTestEnv) IsGpuMachine() bool {
	return strings.Contains(f.VMSize, "a10") || strings.Contains(f.VMSize, "l40s")
}
