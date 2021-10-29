package cmd

import (
	"context"
	"time"

	"github.com/superfly/flyctl/cmd/presenters"
	"github.com/superfly/flyctl/cmdctx"
	"github.com/superfly/flyctl/internal/client"
	"github.com/superfly/flyctl/pkg/logs"
	"github.com/superfly/flyctl/terminal"
	"golang.org/x/sync/errgroup"

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
	ctx := cc.Command.Context()

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

	pollEntries := make(chan logs.LogEntry)
	liveEntries := make(chan logs.LogEntry)

	eg, errCtx := errgroup.WithContext(ctx)
	pollingCtx, pollingCancel := context.WithCancel(errCtx)

	eg.Go(func() error {
		defer close(pollEntries)

		stream, err := logs.NewPollingStream(client, opts)
		if err != nil {
			return err
		}

		for entry := range stream.Stream(pollingCtx, opts) {
			pollEntries <- entry
		}

		return nil
	})

	eg.Go(func() error {
		defer close(liveEntries)

		stream, err := logs.NewNatsStream(errCtx, client, opts)
		if err != nil {
			terminal.Debugf("could not connect to wireguard tunnel, err: %v\n", err)
			terminal.Debug("Falling back to log polling...")
			return nil
		}

		time.Sleep(2 * time.Second)

		pollingCancel()

		for entry := range stream.Stream(errCtx, opts) {
			liveEntries <- entry
		}

		return nil
	})

	presenter := presenters.LogPresenter{}

	eg.Go(func() error {
		for entry := range pollEntries {
			presenter.FPrint(cc.Out, cc.OutputJSON(), entry)
		}
		return nil
	})

	eg.Go(func() error {
		for entry := range liveEntries {
			presenter.FPrint(cc.Out, cc.OutputJSON(), entry)
		}
		return nil
	})

	return eg.Wait()
}
