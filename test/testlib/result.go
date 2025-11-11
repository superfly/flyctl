//go:build integration
// +build integration

package testlib

import (
	"bytes"
	"encoding/json"
	"fmt"

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
}

func (r *FlyctlResult) CmdString() string {
	return r.cmdStr
}

func (r *FlyctlResult) ExitCode() int {
	return r.exitCode
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
	fmt.Printf("DBGCMD: %s\nOUTPUT:\n%s\nSTDERR:\n%s\n", r.cmdStr, r.stdOut.String(), r.stdErr.String())
}
