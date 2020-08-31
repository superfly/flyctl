package cmd

import (
	"fmt"
	"os"

	"github.com/superfly/flyctl/cmdctx"

	"github.com/superfly/flyctl/docstrings"
	"github.com/superfly/flyctl/internal/builds"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/cmd/presenters"
)

func newBuildsCommand() *Command {
	buildsStrings := docstrings.Get("builds")

	cmd := BuildCommandKS(nil, nil, buildsStrings, os.Stdout, requireSession, requireAppName)

	buildsListStrings := docstrings.Get("builds.list")
	BuildCommandKS(cmd, runListBuilds, buildsListStrings, os.Stdout, requireSession, requireAppName)
	buildsLogsStrings := docstrings.Get("builds.logs")
	logs := BuildCommandKS(cmd, runBuildLogs, buildsLogsStrings, os.Stdout, requireSession, requireAppName)
	logs.Command.Args = cobra.ExactArgs(1)

	return cmd
}

func runListBuilds(commandContext *cmdctx.CmdContext) error {
	builds, err := commandContext.Client.API().ListBuilds(commandContext.AppName)
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
