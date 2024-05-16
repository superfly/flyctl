package machine

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flapsutil"
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
		usage = "view [machine-id]"
	)

	cmd := command.New(usage, short, long, runLeaseView,
		command.RequireSession,
		command.LoadAppNameIfPresent,
	)

	cmd.Args = cobra.ArbitraryArgs

	flag.Add(
		cmd,
		flag.App(),
		flag.AppConfig(),
		flag.JSONOutput(),
		selectFlag,
	)

	return cmd
}

func newLeaseClear() *cobra.Command {
	const (
		short = "Clear machine leases"
		long  = short + "\n"
		usage = "clear [machine-id]"
	)

	cmd := command.New(usage, short, long, runLeaseClear,
		command.RequireSession,
		command.LoadAppNameIfPresent,
	)

	cmd.Args = cobra.ArbitraryArgs

	flag.Add(
		cmd,
		flag.App(),
		flag.AppConfig(),
		selectFlag,
	)

	return cmd
}

func runLeaseView(ctx context.Context) (err error) {
	var (
		io   = iostreams.FromContext(ctx)
		args = flag.Args(ctx)
		cfg  = config.FromContext(ctx)
	)

	machines, ctx, err := selectManyMachines(ctx, args)
	if err != nil {
		return err
	}
	flapsClient := flapsutil.ClientFromContext(ctx)

	leases := make(map[string]*fly.MachineLease)

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
		io   = iostreams.FromContext(ctx)
		args = flag.Args(ctx)
	)

	machineIDs, ctx, err := selectManyMachineIDs(ctx, args)
	if err != nil {
		return err
	}
	flapsClient := flapsutil.ClientFromContext(ctx)

	for _, machineID := range machineIDs {
		lease, err := flapsClient.FindLease(ctx, machineID)
		if err != nil {
			if strings.Contains(err.Error(), " lease not found") {
				continue
			}
			return err
		}
		fmt.Fprintf(io.Out, "clearing lease for machine %s\n", machineID)

		if err := flapsClient.ReleaseLease(ctx, machineID, lease.Data.Nonce); err != nil {
			return err
		}
	}
	fmt.Fprintln(io.Out, "Lease(s) cleared")

	return
}
