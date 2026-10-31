//go:build integration
// +build integration

package testlib

import (
	"bytes"
	"encoding/json"
	"fmt"
	"time"

	"github.com/superfly/flyctl/iostreams"
)

// FlyctlResult is the result of running a flyctl command
type FlyctlResult struct {
	t                     testingTWrapper
	argsStr               string
	args                  []string
	cmdStr                string
	testIoStreams         *iostreams.IOStreams
	stdIn, stdOut, stdErr *bytes.Buffer
	exitCode              int
	timedOut              bool
	duration              time.Duration
}

func (r *FlyctlResult) CmdString() string {
	return r.cmdStr
}

func (r *FlyctlResult) ExitCode() int {
	return r.exitCode
}

// TimedOut reports whether the command was killed for exceeding its deadline.
func (r *FlyctlResult) TimedOut() bool {
	return r.timedOut
}

// Duration is how long the command ran, from start until it exited or was killed.
func (r *FlyctlResult) Duration() time.Duration {
	return r.duration
}

func (r *FlyctlResult) StdOut() *bytes.Buffer {
	return r.stdOut
}

func (r *FlyctlResult) StdOutString() string {
	return r.stdOut.String()
}

func (r *FlyctlResult) StdOutJSON(v any) {
	err := json.Unmarshal(r.stdOut.Bytes(), v)
	if err != nil {
		r.t.Fatalf("failed to parse json: %v [output]: %s\n", err, r.stdOut.String())
	}
}

func (r *FlyctlResult) StdErr() *bytes.Buffer {
	return r.stdErr
}

func (r *FlyctlResult) StdErrString() string {
	return r.stdErr.String()
}

func (r *FlyctlResult) AssertSuccessfulExit() {
	r.t.Helper()
	if r.exitCode != 0 {
		r.t.Fatalf("expected successful zero exit code, got %d, for command: %s [stdout]: %s [stderr]: %s", r.exitCode, r.cmdStr, r.stdOut.String(), r.stdErr.String())
	}
}

func (r *FlyctlResult) DebugPrintOutput() {
	r.t.Helper()
	fmt.Printf("DBGCMD: %s\nEXIT: %d after %s\nOUTPUT:\n%s\nSTDERR:\n%s\n", r.cmdStr, r.exitCode, r.duration.Round(time.Millisecond), r.stdOut.String(), r.stdErr.String())
}
