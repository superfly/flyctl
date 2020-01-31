package cmd

import (
	"fmt"
	"github.com/superfly/flyctl/docstrings"
	"os"

	"github.com/skratchdot/open-golang/open"
)

func newDocsCommand() *Command {
	docsStrings := docstrings.Get("docs")
	return BuildCommand(nil, runLaunchDocs, docsStrings.Usage, docsStrings.Short, docsStrings.Long, false, os.Stdout)
}

const docsURL = "https://fly.io/docs/"

func runLaunchDocs(ctx *CmdContext) error {
	fmt.Println("Opening", docsURL)
	return open.Run(docsURL)
}
