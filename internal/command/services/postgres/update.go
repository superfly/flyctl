package postgres

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/internal/app"
	"github.com/superfly/flyctl/internal/client"
	"github.com/superfly/flyctl/internal/command"
	machines "github.com/superfly/flyctl/internal/command/machine"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/pkg/agent"
	"github.com/superfly/flyctl/pkg/flaps"
	"github.com/superfly/flyctl/pkg/flypg"
	"github.com/superfly/flyctl/pkg/iostreams"
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

	agentclient, err := agent.Establish(ctx, client)
	if err != nil {
		return fmt.Errorf("can't establish agent %w", err)
	}

	dialer, err := agentclient.Dialer(ctx, app.Organization.Slug)
	if err != nil {
		return fmt.Errorf("can't build tunnel for %s: %s", app.Organization.Slug, err)
	}

	machines, err := client.ListMachines(ctx, app.ID, "started")
	if err != nil {
		return err
	}

	if len(machines) == 0 {
		return fmt.Errorf("no machines found")
	}

	var (
		leader   *api.Machine
		replicas []*api.Machine
	)

	fmt.Fprintf(io.Out, "Resolving cluster roles\n")

	for _, machine := range machines {
		address := formatAddress(machine)

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
		fmt.Fprintf(io.Out, "  %s: %s\n", machine.ID, role)
	}

	if leader == nil {
		return fmt.Errorf("this cluster has no leader")
	}

	image := fmt.Sprintf("%s:%s", leader.Config.ImageRef.Repository, leader.Config.ImageRef.Tag)

	latest, err := client.GetLatestImageDetails(ctx, image)
	if err != nil {
		return fmt.Errorf("can't get latest image details for %s: %w", image, err)
	}

	fmt.Fprintf(io.Out, "Updating replicas\n")

	for _, replica := range replicas {
		current := replica.Config.ImageRef

		if current.Labels["fly.version"] == latest.Version {
			fmt.Fprintf(io.Out, "  %s: already up to date\n", replica.ID)
			continue
		}

		ref := fmt.Sprintf("%s:%s", latest.Repository, latest.Tag)

		if err := updateMachine(ctx, app, replica, ref, latest.Version); err != nil {
			return fmt.Errorf("can't update %s: %w", replica.ID, err)
		}
	}

	current := leader.Config.ImageRef

	if current.Labels["fly.version"] == latest.Version {
		fmt.Fprintf(io.Out, "%s(leader): already up to date\n", leader.ID)
		return nil
	}

	ref := fmt.Sprintf("%s:%s", latest.Repository, latest.Tag)

	pgclient := flypg.New(app.Name, dialer)

	fmt.Fprintf(io.Out, "Failing over to a new leader\n")

	if err := pgclient.Failover(ctx); err != nil {
		return fmt.Errorf("failed to trigger failover %w", err)
	}

	fmt.Fprintf(io.Out, "Updating leader\n")

	if err := updateMachine(ctx, app, leader, ref, latest.Version); err != nil {
		return err
	}

	fmt.Fprintf(io.Out, "Successfully updated Postgres cluster\n")

	return nil
}

func updateMachine(ctx context.Context, app *api.AppCompact, machine *api.Machine, image, version string) error {
	var io = iostreams.FromContext(ctx)

	flaps, err := flaps.New(ctx, app)
	if err != nil {
		return err
	}

	fmt.Fprintf(io.Out, "Updating machine %s with image %s %s\n", machine.ID, image, version)

	machineConf := machine.Config
	machineConf.Image = image

	input := api.LaunchMachineInput{
		ID:      machine.ID,
		AppID:   app.Name,
		OrgSlug: machine.App.Organization.Slug,
		Region:  machine.Region,
		Config:  &machineConf,
	}

	updated, err := flaps.Update(ctx, input)
	if err != nil {
		return err
	}

	if err := machines.WaitForStart(ctx, flaps, updated); err != nil {
		return err
	}

	return nil
}

func privateIp(machine *api.Machine) string {
	for _, ip := range machine.IPs.Nodes {
		if ip.Family == "v6" && ip.Kind == "privatenet" {
			return ip.IP
		}
	}
	return ""
}

func formatAddress(machine *api.Machine) string {
	addr := privateIp(machine)
	return fmt.Sprintf("[%s]", addr)
}
