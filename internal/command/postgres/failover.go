package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/avast/retry-go/v4"
	"github.com/docker/docker/pkg/ioutils"
	"github.com/mattn/go-colorable"
	"github.com/spf13/cobra"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/agent"
	"github.com/superfly/flyctl/flypg"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/command/apps"
	"github.com/superfly/flyctl/internal/command/ssh"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/flyutil"
	mach "github.com/superfly/flyctl/internal/machine"
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
		flag.Bool{
			Name:        "force",
			Description: "Force a failover even if we can't connect to the active leader",
			Default:     false,
		},
		flag.Bool{
			Name:        "allow-secondary-region",
			Description: "Allow failover to a machine in a secondary region. This is useful when the primary region is unavailable, but the secondary region is still healthy. This is only available for flex machines.",
			Default:     false,
		},
	)

	return cmd
}

func runFailover(ctx context.Context) (err error) {
	var (
		MinPostgresHaVersion         = "0.0.20"
		MinPostgresFlexVersion       = "0.0.3"
		MinPostgresStandaloneVersion = "0.0.7"

		io      = iostreams.FromContext(ctx)
		client  = flyutil.ClientFromContext(ctx)
		appName = appconfig.NameFromContext(ctx)
	)

	app, err := client.GetAppCompact(ctx, appName)
	if err != nil {
		return fmt.Errorf("get app: %w", err)
	}

	if !app.IsPostgresApp() {
		return fmt.Errorf("app %s is not a Postgres app", app.Name)
	}

	ctx, err = apps.BuildContext(ctx, app)
	if err != nil {
		return err
	}

	machines, releaseFunc, err := mach.AcquireAllLeases(ctx)
	defer releaseFunc()
	if err != nil {
		return fmt.Errorf("machines could not be retrieved %w", err)
	}

	if err := hasRequiredVersionOnMachines(app.Name, machines, MinPostgresHaVersion, MinPostgresFlexVersion, MinPostgresStandaloneVersion); err != nil {
		return err
	}

	// You can not failerover for single node postgres
	if len(machines) <= 1 {
		return fmt.Errorf("failover is not available for standalone postgres")
	}

	leader, err := pickLeader(ctx, machines)
	if err != nil {
		return err
	}

	if IsFlex(leader) {
		force := flag.GetBool(ctx, "force")
		allowSecondaryRegion := flag.GetBool(ctx, "allow-secondary-region")

		if failoverErr := flexFailover(ctx, machines, app, force, allowSecondaryRegion); failoverErr != nil {
			if err := handleFlexFailoverFail(ctx, machines); err != nil {
				fmt.Fprintf(io.ErrOut, "Failed to handle failover failure, please manually configure PG cluster primary")
			}
			return fmt.Errorf("Failed to run failover: %s", failoverErr)
		} else {
			return nil
		}
	}

	flapsClient := flapsutil.ClientFromContext(ctx)

	dialer := agent.DialerFromContext(ctx)

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
		retry.Context(ctx), retry.Attempts(30), retry.Delay(1*time.Second), retry.DelayType(retry.FixedDelay),
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

func flexFailover(ctx context.Context, machines []*fly.Machine, app *fly.AppCompact, force, allowSecondaryRegion bool) error {
	if len(machines) < 3 {
		return fmt.Errorf("Not enough machines to meet quorum requirements")
	}

	io := iostreams.FromContext(ctx)

	oldLeader, err := pickLeader(ctx, machines)
	if err != nil {
		return err
	}

	fmt.Fprintf(io.Out, "Performing a failover\n")

	primaryRegion := ""
	primaryCandidates := make([]*fly.Machine, 0)
	secondaryCandidates := make([]*fly.Machine, 0)

	for _, machine := range machines {
		machinePrimaryRegion, ok := machine.Config.Env["PRIMARY_REGION"]
		if !ok || machinePrimaryRegion == "" {
			//  Handle case where PRIMARY_REGION hasn't been set, or is empty.
			return fmt.Errorf("Machine %s does not have a primary region configured", machine.ID)
		}

		if primaryRegion == "" {
			primaryRegion = machinePrimaryRegion
		}

		// Ignore any machines residing outside of the primary region
		if primaryRegion != machinePrimaryRegion {
			return fmt.Errorf("Machines don't agree on a primary region. Cannot safely perform a failover until that's fixed")
		}

		// We don't need to consider the existing leader here.
		if machine == oldLeader {
			continue
		}

		// Ignore any machines residing outside of the primary region
		if primaryRegion == machine.Region {
			primaryCandidates = append(primaryCandidates, machine)
		} else {
			secondaryCandidates = append(secondaryCandidates, machine)
		}
	}

	if primaryRegion == "" {
		return fmt.Errorf("Could not find primary region for app")
	}

	newLeader, err := pickNewLeader(ctx, app, primaryCandidates, secondaryCandidates, allowSecondaryRegion)
	if err != nil {
		return err
	}

	// Stop the leader
	fmt.Println("Stopping current leader... ", oldLeader.ID)
	machineStopInput := fly.StopMachineInput{
		ID:     oldLeader.ID,
		Signal: "SIGINT",
	}

	flapsClient := flapsutil.ClientFromContext(ctx)
	err = flapsClient.Stop(ctx, machineStopInput, oldLeader.LeaseNonce)
	if err != nil {
		return fmt.Errorf("could not stop pg leader %s: %w", oldLeader.ID, err)
	}

	fmt.Println("Starting new leader")
	_, err = flapsClient.Start(ctx, newLeader.ID, newLeader.LeaseNonce)
	if err != nil {
		return err
	}

	cmd := "repmgr standby promote --siblings-follow -f /data/repmgr.conf"
	if force {
		cmd += " -F"
	}

	fmt.Println("Promoting new leader... ", newLeader.ID)
	err = ssh.SSHConnect(&ssh.SSHParams{
		Ctx:      ctx,
		Org:      app.Organization,
		App:      app.Name,
		Username: "postgres",
		Dialer:   agent.DialerFromContext(ctx),
		Cmd:      cmd,
		Stdout:   ioutils.NewWriteCloserWrapper(colorable.NewColorableStdout(), func() error { return nil }),
		Stderr:   ioutils.NewWriteCloserWrapper(colorable.NewColorableStderr(), func() error { return nil }),
		Stdin:    nil,
	}, newLeader.PrivateIP)
	if err != nil {
		return fmt.Errorf("failed to promote machine %s: %s", newLeader.ID, err)
	}

	fmt.Println("Waiting 30 seconds for the old leader to stop...")
	err = flapsClient.Wait(ctx, oldLeader, "stopped", time.Second*30)
	if err != nil {
		return err
	}

	// Restart the old leader
	fmt.Fprintf(io.Out, "Restarting old leader... %s\n", oldLeader.ID)
	mach, err := flapsClient.Start(ctx, oldLeader.ID, oldLeader.LeaseNonce)
	if err != nil {
		return fmt.Errorf("failed to start machine %s: %s", oldLeader.ID, err)
	}
	if mach.Status == "error" {
		return fmt.Errorf("old leader %s could not be started: %s", oldLeader.ID, mach.Message)
	}

	fmt.Printf("Waiting for leadership to swap to %s...\n", newLeader.ID)
	if err := retry.Do(
		func() error {
			oldLeader, err = flapsClient.Get(ctx, newLeader.ID)
			if err != nil {
				return err
			}

			if isLeader(oldLeader) {
				return nil
			} else {
				return fmt.Errorf("Machine %s never became the leader", newLeader.ID)
			}
		},
		retry.Context(ctx), retry.Attempts(60), retry.Delay(time.Second), retry.DelayType(retry.FixedDelay),
	); err != nil {
		return err
	}

	// wait for health checks to pass
	if err := watch.MachinesChecks(ctx, machines); err != nil {
		return fmt.Errorf("failed to wait for health checks to pass: %w", err)
	}

	fmt.Fprintf(io.Out, "Failover complete\n")
	return nil
}

func handleFlexFailoverFail(ctx context.Context, machines []*fly.Machine) (err error) {
	io := iostreams.FromContext(ctx)
	flapsClient := flapsutil.ClientFromContext(ctx)

	leader, err := pickLeader(ctx, machines)
	if err != nil {
		return err
	}

	fmt.Fprintln(io.ErrOut, "Error promoting new leader, restarting existing leader")
	fmt.Println("Waiting for old leader to finish stopping")
	if err := retry.Do(
		func() error {
			leader, err = flapsClient.Get(ctx, leader.ID)
			if err != nil {
				return err
			}

			// Because of the fact that we have to handle a failover fail at any time,
			//  it's possible that the leader hasn't even been stopped yet before failure
			// (due to pickNewLeader failing). If that happens, there's no reason to try and
			// stop that machine again, just to start it.
			if leader.State == "stopped" || leader.State == "started" {
				return nil
			} else if leader.State == "stopping" {
				return fmt.Errorf("Old leader hasn't finished stopping")
			} else {
				return fmt.Errorf("Old leader is in an unexpected state: %s", leader.State)
			}
		},
		retry.Context(ctx), retry.Attempts(60), retry.Delay(time.Second), retry.DelayType(retry.FixedDelay),
	); err != nil {
		return err
	}

	fmt.Println("Clearing existing machine lease...")

	// Clear the existing lease on this machine
	lease, err := flapsClient.FindLease(ctx, leader.ID)
	if err != nil {
		if !strings.Contains(err.Error(), " lease not found") {
			return err
		}
	}
	if err := flapsClient.ReleaseLease(ctx, leader.ID, lease.Data.Nonce); err != nil {
		return err
	}

	fmt.Println("Trying to start old leader")
	// Start the machine again
	leader, err = flapsClient.Get(ctx, leader.ID)
	if err != nil {
		return err
	}

	mach, err := flapsClient.Start(ctx, leader.ID, leader.LeaseNonce)
	if err != nil {
		return err
	}
	if mach.Status == "error" {
		return fmt.Errorf("old leader %s could not be started: %s", leader.ID, mach.Message)
	}

	fmt.Println("Old leader started succesfully")

	return nil
}

func pickNewLeader(ctx context.Context, app *fly.AppCompact, primaryCandidates []*fly.Machine, secondaryCandidates []*fly.Machine, allowSecondaryRegion bool) (*fly.Machine, error) {
	machineReasons := make(map[string]string)

	// We should go for the primary canddiates first, but the secondary candidates are also valid
	var candidates []*fly.Machine
	if allowSecondaryRegion {
		candidates = append(primaryCandidates, secondaryCandidates...)
	} else {
		candidates = primaryCandidates
	}

	for _, machine := range candidates {
		isValid := true
		if isLeader(machine) {
			isValid = false
			machineReasons[machine.ID] = "already leader"
		} else if !machine.AllHealthChecks().AllPassing() {
			isValid = false
			machineReasons[machine.ID] = "1+ health checks are not passing"
		} else if !passesDryRun(ctx, app, machine) {
			isValid = false
			machineReasons[machine.ID] = fmt.Sprintf("Running a dry run of `repmgr standby switchover` failed. Try running `fly ssh console -u postgres -C 'repmgr standby switchover -f /data/repmgr.conf --dry-run' -s -a %s` for more information. This was most likely due to the requirements for quorum not being met.", app.Name)
		}

		if isValid {
			return machine, nil
		}
	}

	err := "no leader could be chosen. Here are the reasons why: \n"
	for machineID, reason := range machineReasons {
		err = fmt.Sprintf("%s%s: %s\n", err, machineID, reason)
	}

	if len(candidates) == 0 && len(secondaryCandidates) > 0 && !allowSecondaryRegion {
		err += "No primary candidates were found, but secondary candidates were found. If you would like to failover to a secondary region, please run this command with the `--allow-secondary-region` flag\n"
	}

	err += "\nplease fix one or more of the above issues, and try again\n"

	return nil, errors.New(err)
}

// Before doing anything that might mess up, it's useful to check if a dry run of the failover command will work, since that allows repmgr to do some checks
func passesDryRun(ctx context.Context, app *fly.AppCompact, machine *fly.Machine) bool {
	err := ssh.SSHConnect(&ssh.SSHParams{
		Ctx:      ctx,
		Org:      app.Organization,
		App:      app.Name,
		Username: "postgres",
		Dialer:   agent.DialerFromContext(ctx),
		Cmd:      "repmgr standby switchover -f /data/repmgr.conf --dry-run",
		Stdout:   nil,
		Stderr:   nil,
		Stdin:    nil,
	}, machine.PrivateIP)

	return err == nil
}
