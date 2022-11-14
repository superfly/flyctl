package machine

import (
	"context"
	"fmt"
	"strconv"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/agent"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/flaps"
	"github.com/superfly/flyctl/internal/app"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/iostreams"
	"github.com/superfly/flyctl/machine"
)

func newRestart() *cobra.Command {
	const (
		short = "Restart one or more Fly machines"
		long  = short + "\n"

		usage = "restart <id> [<id>...]"
	)

	cmd := command.New(usage, short, long, runMachineRestart,
		command.RequireSession,
		command.LoadAppNameIfPresent,
	)

	cmd.Args = cobra.MinimumNArgs(1)

	flag.Add(
		cmd,
		flag.App(),
		flag.AppConfig(),
		flag.String{
			Name:        "signal",
			Shorthand:   "s",
			Description: "Signal to stop the machine with (default: SIGINT)",
		},

		flag.Int{
			Name:        "time",
			Description: "Seconds to wait before killing the machine",
		},
		flag.Bool{
			Name:        "force",
			Description: "Force stop the machine(s)",
		},
	)

	return cmd
}

func runMachineRestart(ctx context.Context) error {
	var (
		io      = iostreams.FromContext(ctx)
		args    = flag.Args(ctx)
		signal  = flag.GetString(ctx, "signal")
		timeout = flag.GetInt(ctx, "time")
		appName = app.NameFromContext(ctx)
		client  = client.FromContext(ctx).API()
	)

	app, err := client.GetAppCompact(ctx, appName)
	if err != nil {
		return fmt.Errorf("could not get app: %w", err)
	}

	ctx, err = buildContext(ctx, app)
	if err != nil {
		return err
	}

	// Resolve flags
	input := &api.RestartMachineInput{
		ForceStop: flag.GetBool(ctx, "force"),
	}

	if timeout != 0 {
		input.Timeout = time.Duration(timeout)
	}

	if signal != "" {
		sig := &api.Signal{}

		s, err := strconv.Atoi(flag.GetString(ctx, "signal"))
		if err != nil {
			return fmt.Errorf("could not get signal %s", err)
		}
		sig.Signal = syscall.Signal(s)
		input.Signal = sig
	}

	flapsClient := flaps.FromContext(ctx)

	// Restart each machine
	for _, machineID := range args {
		m, err := flapsClient.Get(ctx, machineID)
		if err != nil {
			return fmt.Errorf("could not get machine %s: %w", machineID, err)
		}

		if err := machine.Restart(ctx, m, input); err != nil {
			return fmt.Errorf("failed to restart machine %s: %w", m.ID, err)
		}

	}

	return nil
}

func buildContext(ctx context.Context, app *api.AppCompact) (context.Context, error) {
	client := client.FromContext(ctx).API()

	agentclient, err := agent.Establish(ctx, client)
	if err != nil {
		return nil, fmt.Errorf("can't establish agent %w", err)
	}

	dialer, err := agentclient.Dialer(ctx, app.Organization.Slug)
	if err != nil {
		return nil, fmt.Errorf("can't build tunnel for %s: %s", app.Organization.Slug, err)
	}
	ctx = agent.DialerWithContext(ctx, dialer)

	flapsClient, err := flaps.New(ctx, app)
	if err != nil {
		return nil, err
	}

	ctx = flaps.NewContext(ctx, flapsClient)

	return ctx, nil
}
