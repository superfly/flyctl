package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/agent"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/flaps"
	"github.com/superfly/flyctl/flypg"
	"github.com/superfly/flyctl/internal/app"
	"github.com/superfly/flyctl/internal/command"
	machines "github.com/superfly/flyctl/internal/command/machine"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/iostreams"
)

func newUpdate() (cmd *cobra.Command) {
	const (
		long = `Performs a rolling upgrade against the target Postgres cluster.
`
		short = "Updates the Postgres cluster to the latest eligible version"
		usage = "update"
	)

	cmd = command.New(usage, short, long, runUpdate,
		command.RequireSession,
		command.RequireAppName,
	)

	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
	)

	return
}

func runUpdate(ctx context.Context) error {
	appName := app.NameFromContext(ctx)
	client := client.FromContext(ctx).API()
	io := iostreams.FromContext(ctx)

	app, err := client.GetAppCompact(ctx, appName)
	if err != nil {
		return fmt.Errorf("get app: %w", err)
	}

	if app.PlatformVersion != "machines" {
		return fmt.Errorf("this command is only supported for machines clusters.\n For nomad clusters, use `flyctl image update`")
	}

	agentclient, err := agent.Establish(ctx, client)
	if err != nil {
		return fmt.Errorf("can't establish agent %w", err)
	}

	dialer, err := agentclient.Dialer(ctx, app.Organization.Slug)
	if err != nil {
		return fmt.Errorf("can't build tunnel for %s: %s", app.Organization.Slug, err)
	}

	flapsClient, err := flaps.New(ctx, app)
	if err != nil {
		return fmt.Errorf("list of machines could not be retrieved: %w", err)
	}

	machines, err := flapsClient.List(ctx, "started")
	if err != nil {
		return fmt.Errorf("machines could not be retrieved %w", err)
	}

	if len(machines) == 0 {
		return fmt.Errorf("no machines found")
	}

	// Verify that we actually have updates to perform
	fmt.Fprintf(io.Out, "Checking for available updates\n")

	// Track machines that have available updates so we can avoid doing unnecessary work.
	updateList := map[string]*api.ImageVersion{}
	for _, machine := range machines {
		image := fmt.Sprintf("%s:%s", machine.ImageRef.Repository, machine.ImageRef.Tag)

		latest, err := client.GetLatestImageDetails(ctx, image)
		if err != nil {
			return fmt.Errorf("can't get latest image details for %s: %w", image, err)
		}

		if machine.ImageVersion() != latest.Version {
			updateList[machine.ID] = latest
		}
	}

	if len(updateList) == 0 {
		fmt.Fprintf(io.Out, "No updates available...\n")
		return nil
	}

	// Resolve cluster roles
	fmt.Fprintf(io.Out, "Identifying cluster roles\n")
	var (
		leader   *api.Machine
		replicas []*api.Machine
	)

	for _, machine := range machines {
		address := fmt.Sprintf("[%s]", machine.PrivateIP)

		pgclient := flypg.NewFromInstance(address, dialer)
		if err != nil {
			return fmt.Errorf("can't connect to %s: %w", machine.Name, err)
		}

		role, err := pgclient.NodeRole(ctx)
		if err != nil {
			return fmt.Errorf("can't get role for %s: %w", machine.Name, err)
		}

		switch role {
		case "leader":
			leader = machine
		case "replica":
			replicas = append(replicas, machine)
		}
		fmt.Fprintf(io.Out, "  Machine %s: %s\n", machine.ID, role)
	}

	if leader == nil {
		return fmt.Errorf("this cluster has no leader")
	}

	// Acquire leases
	fmt.Fprintf(io.Out, "Attempting to acquire lease(s)\n")
	for _, machine := range machines {
		lease, err := flapsClient.GetLease(ctx, machine.ID, api.IntPointer(40))
		if err != nil {
			return fmt.Errorf("failed to obtain lease: %w", err)
		}
		machine.LeaseNonce = lease.Data.Nonce

		// Ensure lease is released on return
		defer releaseLease(ctx, flapsClient, machine)

		fmt.Fprintf(io.Out, "  Machine %s: %s\n", machine.ID, lease.Status)
	}

	// Update replicas
	fmt.Fprintf(io.Out, "Updating replicas\n")
	for _, replica := range replicas {
		if updateList[replica.ID] == nil {
			fmt.Fprintf(io.Out, "  Machine %s is already running the latest image", replica.ID)
			continue
		}

		ref := updateList[replica.ID]
		image := fmt.Sprintf("%s:%s", ref.Repository, ref.Tag)

		fmt.Fprintf(io.Out, "  Updating machine %s with image %s %s\n", replica.ID, image, ref.Version)
		if err := updateMachine(ctx, app, replica, image); err != nil {
			return fmt.Errorf("can't update %s: %w", replica.ID, err)
		}
	}

	// Update leader
	if updateList[leader.ID] != nil {
		pgclient := flypg.New(app.Name, dialer)

		fmt.Fprintf(io.Out, "Performing a failover\n")
		if err := pgclient.Failover(ctx); err != nil {
			return fmt.Errorf("failed to trigger failover %w", err)
		}

		fmt.Fprintf(io.Out, "Updating leader\n")

		ref := updateList[leader.ID]
		image := fmt.Sprintf("%s:%s", ref.Repository, ref.Tag)

		fmt.Fprintf(io.Out, "  Updating machine %s with image %s %s\n", leader.ID, image, ref.Version)
		if err := updateMachine(ctx, app, leader, image); err != nil {
			return err
		}
	}

	fmt.Fprintf(io.Out, "Postgres cluster has been successfully updated!\n")

	return nil
}

func updateMachine(ctx context.Context, app *api.AppCompact, machine *api.Machine, image string) error {
	flaps, err := flaps.New(ctx, app)
	if err != nil {
		return err
	}

	machineConf := machine.Config
	machineConf.Image = image

	input := api.LaunchMachineInput{
		ID:      machine.ID,
		AppID:   app.Name,
		OrgSlug: app.Organization.Slug,
		Region:  machine.Region,
		Config:  machineConf,
	}

	updated, err := flaps.Update(ctx, input, machine.LeaseNonce)
	if err != nil {
		return err
	}

	if err := machines.WaitForStart(ctx, flaps, updated, time.Minute*5); err != nil {
		return err
	}

	return nil
}

func releaseLease(ctx context.Context, client *flaps.Client, machine *api.Machine) error {
	if err := client.ReleaseLease(ctx, machine.ID, machine.LeaseNonce); err != nil {
		return fmt.Errorf("failed to release lease: %w", err)
	}

	return nil
}
