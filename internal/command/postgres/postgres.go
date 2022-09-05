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
		short = `Manage Postgre clusters.`

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
		newUpdate(),
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

func hasRequiredVersionOnMachines(leader *api.Machine, cluster, standalone string) error {
	// Validate image version to ensure it's compatible with this feature.
	if leader.ImageVersion() == "" || leader.ImageVersion() == "unknown" {
		return fmt.Errorf("command is not compatible with this image")
	}

	imageVersionStr := leader.ImageVersion()[1:]

	imageVersion, err := version.NewVersion(imageVersionStr)
	if err != nil {
		return err
	}

	// Specify compatible versions per repo.
	requiredVersion := &version.Version{}
	if leader.ImageRepository() == "flyio/postgres-standalone" {
		requiredVersion, err = version.NewVersion(standalone)
		if err != nil {
			return err
		}
	}
	if leader.ImageRepository() == "flyio/postgres" {
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

func fetchPGLeader(ctx context.Context, members []*api.Machine) (*api.Machine, error) {
	leader, _, err := nodeRoles(ctx, members)
	if err != nil {
		return nil, err
	}

	if leader == nil {
		return nil, fmt.Errorf("no leader found")
	}
	return leader, nil
}

func nodeRoles(ctx context.Context, machines []*api.Machine) (leader *api.Machine, replicas []*api.Machine, err error) {
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
