package cmd

import (
	"fmt"
	"github.com/superfly/flyctl/docstrings"
	"os"

	"github.com/superfly/flyctl/flyctl"
)

func newVersionCommand() *Command {
	versionStrings := docstrings.Get("version")
	return BuildCommand(nil, runVersion, versionStrings.Usage, versionStrings.Short, versionStrings.Long, false, os.Stdout)
}

func runVersion(ctx *CmdContext) error {
	fmt.Printf("flyctl %s %s %s\n", flyctl.Version, flyctl.Commit, flyctl.BuildDate)
	return nil
}
