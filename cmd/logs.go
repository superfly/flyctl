package cmd

import (
	"github.com/superfly/flyctl/cmdctx"
	"github.com/superfly/flyctl/internal/client"
	"github.com/superfly/flyctl/internal/monitor"

	"github.com/superfly/flyctl/docstrings"
)

func newLogsCommand(client *client.Client) *Command {
	logsStrings := docstrings.Get("logs")
	cmd := BuildCommandKS(nil, runLogs, logsStrings, client, nil, requireSession, requireAppName)

	// TODO: Move flag descriptions into the docStrings
	cmd.AddStringFlag(StringFlagOpts{
		Name:        "instance",
		Shorthand:   "i",
		Description: "Filter by instance ID",
	})
	cmd.AddStringFlag(StringFlagOpts{
		Name:        "region",
		Shorthand:   "r",
		Description: "Filter by region",
	})

	return cmd
}

func runLogs(ctx *cmdctx.CmdContext) error {
	err := monitor.WatchLogs(ctx, ctx.Out, monitor.LogOptions{
		AppName:    ctx.AppName,
		VMID:       ctx.Config.GetString("instance"),
		RegionCode: ctx.Config.GetString("region"),
	})

	return err
}
