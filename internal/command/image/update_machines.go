package image

import (
	"context"
	"fmt"
	"strings"

	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/agent"
	"github.com/superfly/flyctl/flypg"
	"github.com/superfly/flyctl/internal/appsecrets"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flyutil"
	mach "github.com/superfly/flyctl/internal/machine"
	"github.com/superfly/flyctl/iostreams"
)

func updateImageForMachines(ctx context.Context, app *fly.AppCompact) error {
	var (
		io = iostreams.FromContext(ctx)

		autoConfirm      = flag.GetBool(ctx, "yes")
		skipHealthChecks = flag.GetBool(ctx, "skip-health-checks")
	)

	// Acquire leases for all machines
	machines, releaseLeaseFunc, err := mach.AcquireAllLeases(ctx)
	defer releaseLeaseFunc()
	if err != nil {
		return err
	}

	eligible := map[*fly.Machine]fly.MachineConfig{}

	// Loop through machines and compare/confirm changes.
	for _, machine := range machines {
		machineConf := mach.CloneConfig(machine.Config)
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

	minvers, err := appsecrets.GetMinvers(app.Name)
	if err != nil {
		return err
	}
	for machine, machineConf := range eligible {
		input := &fly.LaunchMachineInput{
			Region:            machine.Region,
			Config:            &machineConf,
			SkipHealthChecks:  skipHealthChecks,
			MinSecretsVersion: minvers,
		}
		if err := mach.Update(ctx, machine, input); err != nil {
			return err
		}
	}

	fmt.Fprintln(io.Out, "Machines successfully updated")

	return nil
}

type member struct {
	Machine      *fly.Machine
	TargetConfig fly.MachineConfig
}

func updatePostgresOnMachines(ctx context.Context, app *fly.AppCompact) (err error) {
	var (
		io       = iostreams.FromContext(ctx)
		colorize = io.ColorScheme()
		client   = flyutil.ClientFromContext(ctx)

		autoConfirm = flag.GetBool(ctx, "yes")

		flex = false
	)

	// Acquire leases
	machines, releaseLeaseFunc, err := mach.AcquireAllLeases(ctx)
	defer releaseLeaseFunc()
	if err != nil {
		return err
	}

	// Check if backups are enabled and preserve backup secrets
	backupEnabled, err := isBackupEnabled(ctx, app.Name, client)
	if err != nil {
		return fmt.Errorf("failed to check backup status: %w", err)
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

		machineConf := mach.CloneConfig(machine.Config)

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

	// XXX TODO: use case to think of here is that the machine wasnt provisioned with flyctl.
	minvers, err := appsecrets.GetMinvers(app.Name)
	if err != nil {
		return err
	}

	// Update replicas
	for _, member := range members["replica"] {
		machine := member.Machine
		input := &fly.LaunchMachineInput{
			Region:            machine.Region,
			Config:            &member.TargetConfig,
			MinSecretsVersion: minvers,
		}
		if err := mach.Update(ctx, machine, input); err != nil {
			return err
		}
	}

	// Update any barman nodes
	for _, member := range members["barman"] {
		machine := member.Machine
		input := &fly.LaunchMachineInput{
			Region:            machine.Region,
			Config:            &member.TargetConfig,
			SkipHealthChecks:  true,
			MinSecretsVersion: minvers,
		}
		if err := mach.Update(ctx, machine, input); err != nil {
			return err
		}
	}

	if flex {
		if len(members["primary"]) > 0 {
			primary := members["primary"][0]
			machine := primary.Machine

			input := &fly.LaunchMachineInput{
				Region:            machine.Region,
				Config:            &primary.TargetConfig,
				MinSecretsVersion: minvers,
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
			input := &fly.LaunchMachineInput{
				Region:            machine.Region,
				Config:            &leader.TargetConfig,
				MinSecretsVersion: minvers,
			}
			if err := mach.Update(ctx, machine, input); err != nil {
				return err
			}
		}
	}

	fmt.Fprintln(io.Out, "Postgres cluster has been successfully updated!")

	// If backups were enabled, remind user to redeploy secrets to restore backup configuration
	if backupEnabled {
		fmt.Fprintln(io.Out, colorize.Yellow("⚠️  Backup configuration may need to be restored after image update."))
		fmt.Fprintf(io.Out, colorize.Yellow("   Run `fly secrets deploy -a %s` to ensure backup configuration is active.\n"), app.Name)
	}

	return nil
}

func machineRole(machine *fly.Machine) (role string) {
	role = "unknown"

	for _, check := range machine.Checks {
		if check.Name == "role" {
			if check.Status == fly.Passing {
				role = check.Output
			} else {
				role = "error"
			}
			break
		}
	}
	return role
}

func resolveImage(ctx context.Context, machine fly.Machine) (string, error) {
	var (
		client = flyutil.ClientFromContext(ctx)
		image  = flag.GetString(ctx, "image")
	)

	if image == "" {
		ref := fmt.Sprintf("%s:%s", machine.ImageRef.Repository, machine.ImageRef.Tag)
		latestImage, err := client.GetLatestImageDetails(ctx, ref, machine.ImageVersion())
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

// isBackupEnabled checks if the Postgres app has backups enabled by looking for the backup secret
func isBackupEnabled(ctx context.Context, appName string, client flyutil.Client) (bool, error) {
	secrets, err := client.GetAppSecrets(ctx, appName)
	if err != nil {
		return false, err
	}

	for _, secret := range secrets {
		if secret.Name == "S3_ARCHIVE_CONFIG" {
			return true, nil
		}
	}

	return false, nil
}
