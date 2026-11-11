// Package logs implements the logs command chain.
package logs

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/azazeal/pause"
	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"

	"github.com/superfly/flyctl/iostreams"
	"github.com/superfly/flyctl/logs"

	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/logger"
	"github.com/superfly/flyctl/internal/prompt"
	"github.com/superfly/flyctl/internal/render"
)

func New() (cmd *cobra.Command) {
	const (
		long = `Stream the app's logs, starting with recent lines. The command keeps
streaming until you interrupt it (Ctrl-C); with --no-tail it prints the
recent logs and exits. The app comes from --app or fly.toml.

--machine shows one Machine's logs, and --select lets you pick a Machine
from a list in a terminal. --region shows logs only from Machines in that
region; FLY_REGION, which is set inside Fly Machines, filters the same way
unless --machine is used. --json prints each log entry as a JSON object.`
		short = "View app logs"
	)

	cmd = command.New("logs", short, long, run,
		command.RequireSession,
		command.RequireAppName,
	)

	cmd.Args = cobra.NoArgs

	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
		flag.Region(),
		flag.JSONOutput(),
		flag.String{
			Name:              "machine",
			Shorthand:         "m",
			Description:       "Filter by machine ID",
			Aliases:           []string{"instance"},
			UseAliasShortHand: true,
		},
		flag.Bool{
			Name:        "select",
			Shorthand:   "s",
			Description: "Select a machine from a list",
		},
		flag.Bool{
			Name:        "no-tail",
			Shorthand:   "n",
			Description: "Do not continually stream logs",
		},
	)

	return
}

func run(ctx context.Context) error {
	client := flyutil.ClientFromContext(ctx)

	regionCode := config.FromContext(ctx).Region
	vmid, err := resolveMachineID(ctx, appconfig.NameFromContext(ctx))
	if err != nil {
		return err
	}

	// When filtering by machine ID, ignore region filter since machine IDs are globally unique.
	// This prevents region-locked NATS subscriptions when running on Fly.io machines where
	// FLY_REGION is set, allowing logs to be retrieved from machines in any region.
	if vmid != "" {
		regionCode = ""
	}

	opts := &logs.LogOptions{
		AppName:    appconfig.NameFromContext(ctx),
		RegionCode: regionCode,
		VMID:       vmid,
		NoTail:     flag.GetBool(ctx, "no-tail"),
	}

	flapsClient := flapsutil.ClientFromContext(ctx)

	var eg *errgroup.Group
	eg, ctx = errgroup.WithContext(ctx)

	var streams []<-chan logs.LogEntry
	if opts.NoTail {
		streams = []<-chan logs.LogEntry{
			poll(ctx, eg, client, opts),
		}
	} else {
		pollingCtx, cancelPollingCause := context.WithCancelCause(ctx)
		cancelPolling := func() { cancelPollingCause(fmt.Errorf("log polling replaced by streaming: %w", context.Canceled)) }
		streams = []<-chan logs.LogEntry{
			poll(pollingCtx, eg, client, opts),
			nats(ctx, eg, client, flapsClient, opts, cancelPolling),
		}
	}

	eg.Go(func() error {
		return printStreams(ctx, streams...)
	})

	return eg.Wait()
}

func resolveMachineID(ctx context.Context, appName string) (string, error) {
	machineID := flag.GetString(ctx, "machine")
	if !flag.GetBool(ctx, "select") {
		return machineID, nil
	}
	if flag.IsSpecified(ctx, "machine") {
		return "", errors.New("--machine can't be used with -s/--select")
	}

	machines, err := flapsutil.ClientFromContext(ctx).ListActive(ctx, appName)
	if err != nil {
		return "", fmt.Errorf("could not get a list of machines: %w", err)
	}

	if len(machines) == 0 {
		return "", fmt.Errorf("app %s has no machines", appName)
	}

	if len(machines) == 1 {
		return machines[0].ID, nil
	}

	options := make([]string, 0, len(machines))
	for _, machine := range machines {
		options = append(options, fmt.Sprintf("%s: %s %s", machine.Region, machine.ID, machine.Name))
	}

	var selected int
	if err := prompt.Select(ctx, &selected, "Select a machine:", "", options...); err != nil {
		return "", fmt.Errorf("could not prompt for machine: %w", err)
	}

	return machines[selected].ID, nil
}

func poll(ctx context.Context, eg *errgroup.Group, client flyutil.Client, opts *logs.LogOptions) <-chan logs.LogEntry {
	c := make(chan logs.LogEntry)

	eg.Go(func() (err error) {
		defer close(c)

		if err = logs.Poll(ctx, c, client, opts); errors.Is(err, context.Canceled) {
			// if the parent context is cancelled then the errorgroup will return
			// context.Canceled because nats and/or printStreams will return it.
			err = nil
		}

		return
	})

	return c
}

func nats(ctx context.Context, eg *errgroup.Group, client flyutil.Client, flapsClient flapsutil.FlapsClient, opts *logs.LogOptions, cancelPolling context.CancelFunc) <-chan logs.LogEntry {
	c := make(chan logs.LogEntry)

	eg.Go(func() error {
		defer close(c)

		stream, err := logs.NewNatsStream(ctx, client, flapsClient, opts)
		if err != nil {
			logger := logger.FromContext(ctx)

			logger.Debugf("could not connect to wireguard tunnel: %v\n", err)
			logger.Debug("falling back to log polling...")

			return nil
		}

		// we wait for 2 seconds before canceling the polling context so that
		// we get a few records
		pause.For(ctx, 2*time.Second)
		cancelPolling()

		for entry := range stream.Stream(ctx, opts) {
			c <- entry
		}

		return nil
	})

	return c
}

func printStreams(ctx context.Context, streams ...<-chan logs.LogEntry) error {
	var eg *errgroup.Group
	eg, ctx = errgroup.WithContext(ctx)

	out := iostreams.FromContext(ctx).Out
	json := config.FromContext(ctx).JSONOutput

	for _, stream := range streams {

		eg.Go(func() error {
			return printStream(ctx, out, stream, json)
		})
	}

	return eg.Wait()
}

func printStream(ctx context.Context, w io.Writer, stream <-chan logs.LogEntry, json bool) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case entry, ok := <-stream:
			if !ok {
				return nil
			}

			var err error
			if json {
				err = render.JSON(w, entry)
			} else {
				err = render.LogEntry(w, entry,
					render.HideAllocID(),
					render.RemoveNewlines(),
					render.HideRegion(),
				)
			}

			if err != nil {
				return err
			}
		}
	}
}
