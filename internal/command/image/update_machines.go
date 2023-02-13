package image

import (
	"context"
	"fmt"
	"strings"

	"github.com/superfly/flyctl/agent"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/flypg"
	"github.com/superfly/flyctl/internal/flag"
	mach "github.com/superfly/flyctl/internal/machine"
	"github.com/superfly/flyctl/iostreams"
)

func updateImageForMachines(ctx context.Context, app *api.AppCompact) error {
	var (
		io = iostreams.FromContext(ctx)

		autoConfirm      = flag.GetBool(ctx, "yes")
		skipHealthChecks = flag.GetBool(ctx, "skip-health-checks")
	)

	// Acquire leases for all machines
	machines, releaseLeaseFunc, err := mach.AcquireAllLeases(ctx)
	defer releaseLeaseFunc(ctx, machines)
	if err != nil {
		return err
	}

	eligible := map[*api.Machine]api.MachineConfig{}

	// Loop through machines and compare/confirm changes.
	for _, machine := range machines {
		machineConf, err := mach.CloneConfig(*machine.Config)
		if err != nil {
			return err
		}

		machineConf.Image = machine.FullImageRef()

		image, err := resolveImage(ctx, *machine)
		if err != nil {
			return err
		}

		machineConf.Image = image

		if !autoConfirm {
			confirmed, err := mach.ConfirmConfigChanges(ctx, machine, *machineConf, "")
			if err != nil {
				return err
			}
			if !confirmed {
				continue
			}
		}

		eligible[machine] = *machineConf
	}

	for machine, machineConf := range eligible {
		input := &api.LaunchMachineInput{
			ID:               machine.ID,
			AppID:            app.Name,
			OrgSlug:          app.Organization.Slug,
			Region:           machine.Region,
			Config:           &machineConf,
			SkipHealthChecks: skipHealthChecks,
		}
		if err := mach.Update(ctx, machine, input); err != nil {
			return err
		}
	}

	fmt.Fprintln(io.Out, "Machines successfully updated")

	return nil
}

type member struct {
	Machine      *api.Machine
	TargetConfig api.MachineConfig
}

func updatePostgresOnMachines(ctx context.Context, app *api.AppCompact) (err error) {
	var (
		io       = iostreams.FromContext(ctx)
		colorize = io.ColorScheme()

		autoConfirm = flag.GetBool(ctx, "yes")

		flex = false
	)

	// Acquire leases
	machines, releaseLeaseFunc, err := mach.AcquireAllLeases(ctx)
	defer releaseLeaseFunc(ctx, machines)
	if err != nil {
		return err
	}

	// Identify target images
	members := map[string][]member{}

	prompt := colorize.Bold("The following changes will be applied to all Postgres machines.\n")
	prompt += colorize.Yellow("Machines not running the official Postgres image will be skipped.\n")

	for _, machine := range machines {
		// Ignore any non PG machines
		if !strings.Contains(machine.ImageRef.Repository, "flyio/postgres") {
			continue
		}

		if machine.ImageRef.Labels["fly.pg-manager"] == "repmgr" {
			flex = true
		}

		role := machineRole(machine)

		machineConf, err := mach.CloneConfig(*machine.Config)
		if err != nil {
			return err
		}

		image, err := resolveImage(ctx, *machine)
		if err != nil {
			return err
		}

		// Skip image update if images already match
		if machine.Config.Image == image {
			continue
		}

		machineConf.Image = image

		// Postgres only needs single confirmation.
		if !autoConfirm {
			confirmed, err := mach.ConfirmConfigChanges(ctx, machine, *machineConf, prompt)
			if err != nil {
				switch err.(type) {
				case *mach.ErrNoConfigChangesFound:
					fmt.Printf("Machine %s has no changes", machine.ID)
					continue
				default:
					return err
				}
			}

			if !confirmed {
				return fmt.Errorf("image upgrade aborted")
			}
			autoConfirm = true
		}

		member := member{Machine: machine, TargetConfig: *machineConf}
		members[role] = append(members[role], member)
	}

	if len(members) == 0 {
		fmt.Fprintln(io.Out, colorize.Bold("No changes to apply"))
		return nil
	}

	fmt.Fprintln(io.Out, "Identifying cluster role(s)")
	for role, nodes := range members {
		for _, node := range nodes {
			fmt.Fprintf(io.Out, "  Machine %s: %s\n", colorize.Bold(node.Machine.ID), role)
		}
	}

	// Update replicas
	for _, member := range members["replica"] {
		machine := member.Machine
		input := &api.LaunchMachineInput{
			ID:      machine.ID,
			AppID:   app.Name,
			OrgSlug: app.Organization.Slug,
			Region:  machine.Region,
			Config:  &member.TargetConfig,
		}
		if err := mach.Update(ctx, machine, input); err != nil {
			return err
		}
	}

	if flex {
		if len(members["primary"]) > 0 {
			leader := members["primary"][0]
			machine := leader.Machine

			input := &api.LaunchMachineInput{
				ID:      machine.ID,
				AppID:   app.Name,
				OrgSlug: app.Organization.Slug,
				Region:  machine.Region,
				Config:  &leader.TargetConfig,
			}
			if err := mach.Update(ctx, machine, input); err != nil {
				return err
			}
		}
	} else {

		if len(members["leader"]) > 0 {
			leader := members["leader"][0]
			machine := leader.Machine

			// Verify that we have an in region replica before attempting failover.
			attemptFailover := false
			for _, replica := range members["replicas"] {
				if replica.Machine.Region == leader.Machine.Region {
					attemptFailover = true
					break
				}
			}

			// Skip failover if we don't have any replicas.
			if attemptFailover {
				dialer := agent.DialerFromContext(ctx)
				pgclient := flypg.NewFromInstance(machine.PrivateIP, dialer)
				fmt.Fprintf(io.Out, "Attempting to failover %s\n", colorize.Bold(machine.ID))

				if err := pgclient.Failover(ctx); err != nil {
					fmt.Fprintln(io.Out, colorize.Red(fmt.Sprintf("failed to perform failover: %s", err.Error())))
				}
			}

			// Update leader
			input := &api.LaunchMachineInput{
				ID:      machine.ID,
				AppID:   app.Name,
				OrgSlug: app.Organization.Slug,
				Region:  machine.Region,
				Config:  &leader.TargetConfig,
			}
			if err := mach.Update(ctx, machine, input); err != nil {
				return err
			}
		}
	}

	fmt.Fprintln(io.Out, "Postgres cluster has been successfully updated!")

	return nil
}

func machineRole(machine *api.Machine) (role string) {
	role = "unknown"

	for _, check := range machine.Checks {
		if check.Name == "role" {
			if check.Status == "passing" {
				role = check.Output
			} else {
				role = "error"
			}
			break
		}
	}
	return role
}

func resolveImage(ctx context.Context, machine api.Machine) (string, error) {
	var (
		client = client.FromContext(ctx).API()
		image  = flag.GetString(ctx, "image")
	)

	if image == "" {
		ref := fmt.Sprintf("%s:%s", machine.ImageRef.Repository, machine.ImageRef.Tag)
		latestImage, err := client.GetLatestImageDetails(ctx, ref)
		if err != nil && !strings.Contains(err.Error(), "Unknown repository") {
			return "", err
		}

		if latestImage != nil {
			image = latestImage.FullImageRef()
		}

		if image == "" {
			image = machine.FullImageRef()
		}
	}

	return image, nil
}
