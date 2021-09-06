package cmd

import (
	"github.com/superfly/flyctl/cmd/presenters"
	"github.com/superfly/flyctl/cmdctx"
	"github.com/superfly/flyctl/internal/client"
	"github.com/superfly/flyctl/pkg/logs"
	"github.com/superfly/flyctl/terminal"

	"github.com/superfly/flyctl/docstrings"
)

func newLogsCommand(client *client.Client) *Command {
	logsStrings := docstrings.Get("logs")
	cmd := BuildCommandKS(nil, runLogs, logsStrings, client, requireSession, requireAppName)

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

func runLogs(cc *cmdctx.CmdContext) error {
	ctx := createCancellableContext()

	client := cc.Client.API()

	opts := &logs.LogOptions{
		AppName:    cc.Config.GetString("app"),
		RegionCode: cc.Config.GetString("region"),
		VMID:       cc.Config.GetString("instance"),
	}

	stream, err := logs.NewNatsStream(client, opts)

	if err != nil {
		terminal.Debugf("could not connect to wireguard tunnel, err: %v\n", err)
		terminal.Debug("Falling back to log polling...")

		stream, err = logs.NewPollingStream(client)
		if err != nil {
			return err
		}
	}

	presenter := presenters.LogPresenter{}

	entries := stream.Stream(ctx, opts)

	for {
		select {
		case <-ctx.Done():
			return stream.Err()
		case entry := <-entries:
			presenter.FPrint(cc.Out, false, entry)
		}
	}
}
