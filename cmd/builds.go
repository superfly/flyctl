package cmd

import (
	"fmt"

	"github.com/superfly/flyctl/cmdctx"

	"github.com/superfly/flyctl/docstrings"
	"github.com/superfly/flyctl/internal/builds"
	"github.com/superfly/flyctl/internal/client"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/cmd/presenters"
)

func newBuildsCommand(client *client.Client) *Command {
	buildsStrings := docstrings.Get("builds")

	cmd := BuildCommandKS(nil, nil, buildsStrings, client, requireSession, requireAppName)

	buildsListStrings := docstrings.Get("builds.list")
	BuildCommandKS(cmd, runListBuilds, buildsListStrings, client, requireSession, requireAppName)
	buildsLogsStrings := docstrings.Get("builds.logs")
	logs := BuildCommandKS(cmd, runBuildLogs, buildsLogsStrings, client, requireSession, requireAppName)
	logs.Command.Args = cobra.ExactArgs(1)

	return cmd
}

func runListBuilds(commandContext *cmdctx.CmdContext) error {
	ctx := createCancellableContext()

	builds, err := commandContext.Client.API().ListBuilds(ctx, commandContext.AppName)
	if err != nil {
		return err
	}

	return commandContext.Frender(cmdctx.PresenterOption{Presentable: &presenters.Builds{Builds: builds}})
}

func runBuildLogs(cc *cmdctx.CmdContext) error {
	ctx := createCancellableContext()
	buildID := cc.Args[0]

	logs := builds.NewBuildMonitor(buildID, cc.Client.API())

	// TODO: Need to consider what is appropriate to output with JSON set
	for line := range logs.Logs(ctx) {
		fmt.Println(line)
	}

	if err := logs.Err(); err != nil {
		return err
	}

	return nil
}
