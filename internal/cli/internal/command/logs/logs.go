package logs

import (
	"context"
	"time"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/cmd/presenters"
	"github.com/superfly/flyctl/internal/cli/internal/app"
	"github.com/superfly/flyctl/internal/cli/internal/command"
	"github.com/superfly/flyctl/internal/cli/internal/config"
	"github.com/superfly/flyctl/internal/cli/internal/flag"
	"github.com/superfly/flyctl/internal/client"
	"github.com/superfly/flyctl/pkg/iostreams"
	"github.com/superfly/flyctl/pkg/logs"
	"github.com/superfly/flyctl/terminal"
	"golang.org/x/sync/errgroup"

	"github.com/superfly/flyctl/docstrings"
)

func New() *cobra.Command {

	logsStrings := docstrings.Get("logs")

	cmd := command.New("logs", logsStrings.Short, logsStrings.Long, run, command.RequireSession, command.RequireAppName)

	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
	)

	flag.Add(cmd, flag.String{
		Name:        "instance",
		Shorthand:   "i",
		Description: "Filter by instance ID",
	})

	flag.Add(cmd, flag.String{
		Name:        "region",
		Shorthand:   "r",
		Description: "Filter by region",
	})

	return cmd
}

func run(ctx context.Context) (err error) {
	client := client.FromContext(ctx).API()
	appName := app.NameFromContext(ctx)
	jsonOutput := config.FromContext(ctx).JSONOutput
	io := iostreams.FromContext(ctx)

	app, err := client.GetApp(ctx, appName)
	if err != nil {
		return err
	}

	opts := &logs.LogOptions{
		AppName:    app.Name,
		RegionCode: flag.GetString(ctx, "region"),
		VMID:       flag.GetString(ctx, "instance"),
	}

	pollEntries := make(chan logs.LogEntry)
	liveEntries := make(chan logs.LogEntry)

	eg, errCtx := errgroup.WithContext(ctx)
	pollingCtx, pollingCancel := context.WithCancel(errCtx)

	eg.Go(func() error {
		defer close(pollEntries)

		stream, err := logs.NewPollingStream(ctx, client, opts)
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
			presenter.FPrint(io.Out, jsonOutput, entry)
		}
		return nil
	})

	eg.Go(func() error {
		for entry := range liveEntries {
			presenter.FPrint(io.Out, jsonOutput, entry)
		}
		return nil
	})

	return eg.Wait()
}
