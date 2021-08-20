package cmd

import (
	"fmt"

	"github.com/superfly/flyctl/cmdctx"
	"github.com/superfly/flyctl/internal/client"

	"github.com/superfly/flyctl/docstrings"

	"github.com/skratchdot/open-golang/open"
)

func newDocsCommand(client *client.Client) *Command {
	docsStrings := docstrings.Get("docs")
	return BuildCommand(nil, runLaunchDocs, docsStrings.Usage, docsStrings.Short, docsStrings.Long, client, nil)
}

const docsURL = "https://fly.io/docs/"

func runLaunchDocs(ctx *cmdctx.CmdContext) error {
	fmt.Println("Opening", docsURL)
	return open.Run(docsURL)
}
