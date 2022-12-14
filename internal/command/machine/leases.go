package machine

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/flaps"
	"github.com/superfly/flyctl/internal/app"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/command/apps"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/render"
	"github.com/superfly/flyctl/iostreams"
)

func newLeases() *cobra.Command {
	const (
		short = "Manage machine leases"
		long  = short + "\n"
		usage = "leases <command>"
	)

	cmd := command.New(usage, short, long, nil)

	cmd.Aliases = []string{"lease"}

	cmd.Args = cobra.NoArgs

	cmd.AddCommand(
		newLeaseView(),
		newLeaseClear(),
	)

	return cmd
}

func newLeaseView() *cobra.Command {
	const (
		short = "View machine leases"
		long  = short + "\n"
		usage = "view"
	)

	cmd := command.New(usage, short, long, runLeaseView,
		command.RequireSession,
		command.LoadAppNameIfPresent,
	)

	// at least one arg is required but we can accept a list of machine ids
	cmd.Args = cobra.MinimumNArgs(1)

	flag.Add(
		cmd,
		flag.App(),
		flag.AppConfig(),
	)

	return cmd
}

func newLeaseClear() *cobra.Command {
	const (
		short = "Clear machine leases"
		long  = short + "\n"
		usage = "clear"
	)

	cmd := command.New(usage, short, long, runLeaseClear,
		command.RequireSession,
		command.LoadAppNameIfPresent,
	)

	// at least one arg is required but we can accept a list of machine ids
	cmd.Args = cobra.MinimumNArgs(1)

	flag.Add(
		cmd,
		flag.App(),
		flag.AppConfig(),
	)

	return cmd
}

func runLeaseView(ctx context.Context) (err error) {
	var (
		io      = iostreams.FromContext(ctx)
		args    = flag.Args(ctx)
		cfg     = config.FromContext(ctx)
		appName = app.NameFromContext(ctx)
		client  = client.FromContext(ctx).API()
	)

	app, err := client.GetAppCompact(ctx, appName)
	if err != nil {
		return fmt.Errorf("could not get app: %w", err)
	}

	ctx, err = apps.BuildContext(ctx, app)
	if err != nil {
		return
	}

	flapsClient := flaps.FromContext(ctx)

	var machines []*api.Machine
	// Resolve machines
	for _, machineID := range args {
		machine, err := flapsClient.Get(ctx, machineID)
		if err != nil {
			return fmt.Errorf("could not get machine %s: %w", machineID, err)
		}
		machines = append(machines, machine)
	}

	var leases = make(map[string]*api.MachineLease)

	for _, machine := range machines {
		lease, err := flapsClient.FindLease(ctx, machine.ID)
		if err != nil {
			if strings.Contains(err.Error(), " lease not found") {
				continue
			}
			return err
		}
		if lease == nil {
			continue
		}

		leases[machine.ID] = lease
	}

	if cfg.JSONOutput {
		return render.JSON(io.Out, leases)
	}

	if len(leases) == 0 {
		fmt.Fprintln(io.Out, "No leases found")
		return nil
	}

	rows := [][]string{}

	for machine, lease := range leases {
		expires := time.Unix(lease.Data.ExpiresAt, 0).Format(time.RFC3339)

		rows = append(rows, []string{
			machine,
			lease.Data.Nonce,
			lease.Data.Owner,
			lease.Status,
			expires,
		})
	}

	_ = render.Table(io.Out, "", rows, "Machine", "Nonce", "Status", "Owner", "Expires")

	return
}

func runLeaseClear(ctx context.Context) (err error) {
	var (
		io      = iostreams.FromContext(ctx)
		args    = flag.Args(ctx)
		appName = app.NameFromContext(ctx)
		client  = client.FromContext(ctx).API()
	)

	app, err := client.GetAppCompact(ctx, appName)
	if err != nil {
		return fmt.Errorf("could not get app: %w", err)
	}

	ctx, err = apps.BuildContext(ctx, app)
	if err != nil {
		return
	}

	flapsClient := flaps.FromContext(ctx)

	for _, machineID := range args {
		lease, err := flapsClient.FindLease(ctx, machineID)
		if err != nil {
			if strings.Contains(err.Error(), " lease not found") {
				continue
			}
			return err
		}
		fmt.Fprintf(io.Out, "clearing lease for machine %s\n", machineID)

		if err := flapsClient.ReleaseLease(ctx, lease.Data.Nonce, machineID); err != nil {
			return err
		}
	}
	fmt.Fprintln(io.Out, "Lease(s) cleared")

	return
}
