package cmd

import (
	"fmt"
	"github.com/superfly/flyctl/cmdctx"
	"os"

	"github.com/superfly/flyctl/docstrings"
	"github.com/superfly/flyctl/internal/builds"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/cmd/presenters"
)

func newBuildsCommand() *Command {
	buildsStrings := docstrings.Get("builds")

	cmd := &Command{
		Command: &cobra.Command{
			Use:   buildsStrings.Usage,
			Short: buildsStrings.Short,
			Long:  buildsStrings.Long,
		},
	}

	buildsListStrings := docstrings.Get("builds.list")
	BuildCommand(cmd, runListBuilds, buildsListStrings.Usage, buildsListStrings.Short, buildsListStrings.Long, os.Stdout, requireSession, requireAppName)
	buildsLogsStrings := docstrings.Get("builds.logs")
	logs := BuildCommand(cmd, runBuildLogs, buildsLogsStrings.Usage, buildsLogsStrings.Short, buildsLogsStrings.Long, os.Stdout, requireSession, requireAppName)
	logs.Command.Args = cobra.ExactArgs(1)

	return cmd
}

func runListBuilds(ctx *cmdctx.CmdContext) error {
	builds, err := ctx.Client.API().ListBuilds(ctx.AppName)
	if err != nil {
		return err
	}

	return ctx.Frender(ctx.Out, cmdctx.PresenterOption{Presentable: &presenters.Builds{Builds: builds}})
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
