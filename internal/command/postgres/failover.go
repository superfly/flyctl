package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/avast/retry-go/v4"
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/agent"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/flaps"
	"github.com/superfly/flyctl/flypg"
	"github.com/superfly/flyctl/internal/app"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/watch"
	"github.com/superfly/flyctl/iostreams"
)

func newFailover() *cobra.Command {
	const (
		short = "Failover to a new primary"
		long  = short + "\n"
		usage = "failover"
	)

	cmd := command.New(usage, short, long, runFailover,
		command.RequireSession,
		command.RequireAppName,
	)

	flag.Add(
		cmd,
		flag.App(),
		flag.AppConfig(),
	)

	return cmd
}

func runFailover(ctx context.Context) (err error) {
	var (
		MinPostgresHaVersion = "0.0.20"
		io                   = iostreams.FromContext(ctx)
		client               = client.FromContext(ctx).API()
		appName              = app.NameFromContext(ctx)
	)

	app, err := client.GetAppCompact(ctx, appName)
	if err != nil {
		return fmt.Errorf("get app: %w", err)
	}

	if !app.IsPostgresApp() {
		return fmt.Errorf("app %s is not a Postgres app", app.Name)
	}

	if app.PlatformVersion != "machines" {
		return fmt.Errorf("failerover is only supported for machines apps")
	}

	agentclient, err := agent.Establish(ctx, client)
	if err != nil {
		return fmt.Errorf("error establishing agent: %w", err)
	}

	dialer, err := agentclient.Dialer(ctx, app.Organization.Slug)
	if err != nil {
		return fmt.Errorf("flaps: can't build tunnel for %s: %w", app.Organization.Slug, err)
	}
	ctx = agent.DialerWithContext(ctx, dialer)

	flapsClient, err := flaps.New(ctx, app)
	if err != nil {
		return fmt.Errorf("list of machines could not be retrieved: %w", err)
	}
	ctx = flaps.NewContext(ctx, flapsClient)

	machines, err := flapsClient.ListActive(ctx)
	if err != nil {
		return fmt.Errorf("machines could not be retrieved %w", err)
	}

	if err := machineVersionCompatible(machines, MinPostgresHaVersion, MinPostgresHaVersion); err != nil {
		return err
	}

	// You can not failerover for single node postgres
	if len(machines) <= 1 {
		return fmt.Errorf("failover is not available for standalone postgres")
	}

	// acquire cluster wide lock
	for _, machine := range machines {
		lease, err := flapsClient.GetLease(ctx, machine.ID, api.IntPointer(40))
		if err != nil {
			return fmt.Errorf("failed to obtain lease: %w", err)
		}
		machine.LeaseNonce = lease.Data.Nonce

		// Ensure lease is released on return
		defer flapsClient.ReleaseLease(ctx, machine.ID, machine.LeaseNonce)
	}

	leader, err := pickLeader(ctx, machines)
	if err != nil {
		return err
	}

	pgclient := flypg.NewFromInstance(leader.PrivateIP, dialer)
	fmt.Fprintf(io.Out, "Performing a failover\n")
	if err := pgclient.Failover(ctx); err != nil {
		return fmt.Errorf("failed to trigger failover %w", err)
	}

	// Wait until the leader lost its role
	if err := retry.Do(
		func() error {
			var err error
			leader, err = flapsClient.Get(ctx, leader.ID)
			if err != nil {
				return err
			} else if machineRole(leader) == "leader" {
				return fmt.Errorf("%s hasn't lost its leader role", leader.ID)
			}
			return nil
		},
		retry.Context(ctx), retry.Attempts(30), retry.Delay(time.Second), retry.DelayType(retry.FixedDelay),
	); err != nil {
		return err
	}

	// wait for health checks to pass
	if err := watch.MachinesChecks(ctx, machines); err != nil {
		return fmt.Errorf("failed to wait for health checks to pass: %w", err)
	}

	fmt.Fprintf(io.Out, "Failover complete\n")
	return
}
