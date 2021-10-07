package cmd

import (
	"time"

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

	app, err := client.GetApp(cc.AppName)
	if err != nil {
		return err
	}

	opts := &logs.LogOptions{
		AppName:    app.Name,
		RegionCode: cc.Config.GetString("region"),
		VMID:       cc.Config.GetString("instance"),
	}

	entries := make(chan logs.LogEntry, 2)

	stop := make(chan struct{})

	go func() {
		stream, err := logs.NewNatsStream(client, opts)
		if err != nil {
			terminal.Debugf("could not connect to wireguard tunnel, err: %v\n", err)
			terminal.Debug("Falling back to log polling...")
			return
		}

		time.Sleep(5 * time.Second)

		stop <- struct{}{}

		for entry := range stream.Stream(ctx, opts) {
			entries <- entry
		}
	}()

	go func() {
		stream, err := logs.NewPollingStream(client, opts)
		if err != nil {
			return
		}
		for {
			select {
			case entry := <-stream.Stream(ctx, opts):
				entries <- entry
			case <-stop:
				return
			}
		}

	}()

	presenter := presenters.LogPresenter{}

	for {
		select {
		case <-ctx.Done():
			close(entries)
			return nil
		case entry := <-entries:
			presenter.FPrint(cc.Out, cc.OutputJSON(), entry)
		}
	}
}
