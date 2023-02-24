//go:build integration
// +build integration

package testlib

import (
	"bytes"
	"fmt"

	"github.com/superfly/flyctl/iostreams"
)

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

func (r *FlyctlResult) StdErr() *bytes.Buffer {
	return r.stdErr
}

func (r *FlyctlResult) AssertSuccessfulExit() {
	r.t.Helper()
	if r.exitCode != 0 {
		r.t.Fatalf("expected successful zero exit code, got %d, for command: %s [stdout]: %s [strderr]: %s", r.exitCode, r.cmdStr, r.stdOut.String(), r.stdErr.String())
	}
}

func (r *FlyctlResult) DebugPrintOutput() {
	r.t.Helper()
	fmt.Printf("DBGCMD: %s\nOUTPUT:\n%s\n", r.cmdStr, r.stdOut.String())
}
