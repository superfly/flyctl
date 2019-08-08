package cmd

import (
	"fmt"
	"os"

	"github.com/superfly/flyctl/flyctl"
)

func newVersionCommand() *Command {
	return BuildCommand(nil, runVersion, "version", "show flyctl version information", os.Stdout, false)
}

func runVersion(ctx *CmdContext) error {
	fmt.Printf("flyctl %s %s %s\n", flyctl.Version, flyctl.Commit, flyctl.BuildDate)
	return nil
}
