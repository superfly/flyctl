package cmd

import (
	"fmt"
	"os"

	"github.com/skratchdot/open-golang/open"
)

func newDocsCommand() *Command {
	return BuildCommand(nil, runLaunchDocs, "docs", "view documentation", os.Stdout, false)
}

const docsURL = "https://fly.io/docs/future/"

func runLaunchDocs(ctx *CmdContext) error {
	fmt.Println("Opening", docsURL)
	return open.Run(docsURL)
}
