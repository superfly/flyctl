package cmd

import (
	"fmt"
	"github.com/superfly/flyctl/docstrings"
	"os"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/cmd/presenters"
	"github.com/superfly/flyctl/src/flyctl"
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
	BuildCommand(cmd, runListBuilds, buildsListStrings.Usage, buildsListStrings.Short, buildsListStrings.Long, true, os.Stdout, requireAppName)
	buildsLogsStrings := docstrings.Get("builds.logs")
	logs := BuildCommand(cmd, runBuildLogs, buildsLogsStrings.Usage, buildsLogsStrings.Short, buildsLogsStrings.Long, true, os.Stdout, requireAppName)
	logs.Command.Args = cobra.ExactArgs(1)

	return cmd
}

func runListBuilds(ctx *CmdContext) error {
	builds, err := ctx.FlyClient.ListBuilds(ctx.AppName)
	if err != nil {
		return err
	}

	return ctx.Render(&presenters.Builds{Builds: builds})
}

func runBuildLogs(ctx *CmdContext) error {
	buildID := ctx.Args[0]

	logs := flyctl.NewBuildLogStream(buildID, ctx.FlyClient)

	for line := range logs.Fetch() {
		fmt.Println(line)
	}

	if err := logs.Err(); err != nil {
		return err
	}

	return nil
}
