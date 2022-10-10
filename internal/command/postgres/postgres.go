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

func hasRequiredVersionOnMachines(machines []*api.Machine, cluster, standalone string) error {
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

func machinesNodeRoles(ctx context.Context, machines []*api.Machine) (leader *api.Machine, replicas []*api.Machine, err error) {
	var dialer = agent.DialerFromContext(ctx)

	for _, machine := range machines {
		address := fmt.Sprintf("[%s]", machine.PrivateIP)

		pgclient := flypg.NewFromInstance(address, dialer)
		if err != nil {
			return nil, nil, fmt.Errorf("can't connect to %s: %w", machine.Name, err)
		}

		role, err := pgclient.NodeRole(ctx)
		if err != nil {
			return nil, nil, fmt.Errorf("can't get role for %s: %w", machine.Name, err)
		}

		switch role {
		case "leader":
			leader = machine
		case "replica":
			replicas = append(replicas, machine)
		}
	}
	return leader, replicas, nil
}

func nomadNodeRoles(ctx context.Context, allocs []*api.AllocationStatus) (leader *api.AllocationStatus, replicas []*api.AllocationStatus, err error) {
	var dialer = agent.DialerFromContext(ctx)

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
