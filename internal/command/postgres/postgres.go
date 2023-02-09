package postgres

import (
	"context"
	"fmt"

	"github.com/hashicorp/go-version"
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/agent"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/flypg"
	"github.com/superfly/flyctl/internal/command"
	mach "github.com/superfly/flyctl/internal/machine"
)

func New() *cobra.Command {
	const (
		short = `Manage Postgres clusters.`

		long = short + "\n"
	)

	cmd := command.New("postgres", short, long, nil)

	cmd.Aliases = []string{"pg"}

	cmd.AddCommand(
		newAttach(),
		newConfig(),
		newConnect(),
		newCreate(),
		newDb(),
		newDetach(),
		newList(),
		newRestart(),
		newUsers(),
		newFailover(),
		newNomadToMachines(),
		newAddFlycast(),
	)

	return cmd
}

func hasRequiredVersionOnNomad(app *api.AppCompact, cluster, standalone string) error {
	// Validate image version to ensure it's compatible with this feature.
	if app.ImageDetails.Version == "" || app.ImageDetails.Version == "unknown" {
		return fmt.Errorf("command is not compatible with this image")
	}

	imageVersionStr := app.ImageDetails.Version[1:]
	imageVersion, err := version.NewVersion(imageVersionStr)
	if err != nil {
		return err
	}

	// Specify compatible versions per repo.
	requiredVersion := &version.Version{}
	if app.ImageDetails.Repository == "flyio/postgres-standalone" {
		requiredVersion, err = version.NewVersion(standalone)
		if err != nil {
			return err
		}
	}
	if app.ImageDetails.Repository == "flyio/postgres" {
		requiredVersion, err = version.NewVersion(cluster)
		if err != nil {
			return err
		}
	}

	if requiredVersion == nil {
		return fmt.Errorf("unable to resolve image version")
	}

	if imageVersion.LessThan(requiredVersion) {
		return fmt.Errorf(
			"image version is not compatible. (Current: %s, Required: >= %s)\n"+
				"Please run 'flyctl image show' and update to the latest available version",
			imageVersion, requiredVersion.String())
	}

	return nil
}

func hasRequiredVersionOnMachines(machines []*api.Machine, cluster, flex, standalone string) error {
	for _, machine := range machines {
		// Validate image version to ensure it's compatible with this feature.
		if machine.ImageVersion() == "" || machine.ImageVersion() == "unknown" {
			return fmt.Errorf("command is not compatible with this image")
		}

		imageVersionStr := machine.ImageVersion()[1:]

		imageVersion, err := version.NewVersion(imageVersionStr)
		if err != nil {
			return err
		}

		// Specify compatible versions per repo.
		requiredVersion := &version.Version{}
		if machine.ImageRepository() == "flyio/postgres-standalone" {
			requiredVersion, err = version.NewVersion(standalone)
			if err != nil {
				return err
			}
		}
		if machine.ImageRepository() == "flyio/postgres" {
			requiredVersion, err = version.NewVersion(cluster)
			if err != nil {
				return err
			}
		}

		if machine.ImageRepository() == "flyio/postgres-timescaledb" {
			requiredVersion, err = version.NewVersion(cluster)
			if err != nil {
				return err
			}
		}

		if IsFlex(machine) {
			requiredVersion, err = version.NewVersion(flex)
			if err != nil {
				return err
			}
		}

		if requiredVersion == nil {
			return fmt.Errorf("unable to resolve image version")
		}

		if imageVersion.LessThan(requiredVersion) {
			return fmt.Errorf(
				"%s is running an incompatible image version. (Current: %s, Required: >= %s)\n"+
					"Please run 'flyctl pg update' to update to the latest available version",
				machine.ID, imageVersion, requiredVersion.String())
		}

	}
	return nil
}

func IsFlex(machine *api.Machine) bool {
	if machine.ImageRef.Labels["fly.manager"] == "repmgr" {
		return true
	}

	return false
}

func machinesNodeRoles(ctx context.Context, machines []*api.Machine) (leader *api.Machine, replicas []*api.Machine) {
	for _, machine := range machines {
		role := machineRole(machine)

		switch role {
		case "leader", "primary":
			leader = machine
		case "replica", "standby":
			replicas = append(replicas, machine)
		default:
			replicas = append(replicas, machine)
		}
	}
	return leader, replicas
}

func nomadNodeRoles(ctx context.Context, allocs []*api.AllocationStatus) (leader *api.AllocationStatus, replicas []*api.AllocationStatus, err error) {
	dialer := agent.DialerFromContext(ctx)

	for _, alloc := range allocs {
		pgclient := flypg.NewFromInstance(alloc.PrivateIP, dialer)
		if err != nil {
			return nil, nil, fmt.Errorf("can't connect to %s: %w", alloc.ID, err)
		}

		role, err := pgclient.NodeRole(ctx)
		if err != nil {
			return nil, nil, fmt.Errorf("can't get role for %s: %w", alloc.ID, err)
		}

		switch role {
		case "leader":
			leader = alloc
		case "replica":
			replicas = append(replicas, alloc)
		}
	}
	return leader, replicas, nil
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

func leaderIpFromNomadInstances(ctx context.Context, addrs []string) (string, error) {
	dialer := agent.DialerFromContext(ctx)
	for _, addr := range addrs {
		pgclient := flypg.NewFromInstance(addr, dialer)
		role, err := pgclient.NodeRole(ctx)
		if err != nil {
			return "", fmt.Errorf("can't get role for %s: %w", addr, err)
		}

		if role == "leader" || role == "primary" {
			return addr, nil
		}
	}
	return "", fmt.Errorf("no instances found with leader role")
}

func pickLeader(ctx context.Context, machines []*api.Machine) (*api.Machine, error) {
	for _, machine := range machines {
		if machineRole(machine) == "leader" || machineRole(machine) == "primary" {
			return machine, nil
		}
	}
	return nil, fmt.Errorf("no active leader found")
}

func UnregisterMember(ctx context.Context, app *api.AppCompact, machine *api.Machine) error {
	machines, err := mach.ListActive(ctx)
	if err != nil {
		return err
	}

	leader, err := pickLeader(ctx, machines)
	if err != nil {
		return err
	}

	cmd, err := flypg.NewCommand(ctx, app)
	if err != nil {
		return err
	}

	if err := cmd.UnregisterMember(ctx, leader.PrivateIP, machine.PrivateIP); err != nil {
		return err
	}

	return nil
}
