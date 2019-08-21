package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/cmd/presenters"
	"github.com/superfly/flyctl/flyctl"
)

func newBuildsCommand() *Command {
	cmd := &Command{
		Command: &cobra.Command{
			Use:   "builds",
			Short: "interact with builds",
		},
	}

	BuildCommand(cmd, runListBuilds, "list", "list builds", os.Stdout, true, requireAppName)
	logs := BuildCommand(cmd, runBuildLogs, "logs", "show build logs", os.Stdout, true, requireAppName)
	logs.Command.Args = cobra.ExactArgs(1)

	return cmd
}

func runListBuilds(ctx *CmdContext) error {
	builds, err := ctx.FlyClient.ListBuilds(ctx.AppName())
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
