package postgres

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/internal/cli/internal/app"
	"github.com/superfly/flyctl/internal/cli/internal/command"
	"github.com/superfly/flyctl/internal/client"
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

	app, err := client.GetApp(ctx, appName)
	if err != nil {
		return fmt.Errorf("get app: %w", err)
	}

	cli, err := flaps.New(ctx, app)
	if err != nil {
		return err
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

	imageRef, err := client.GetLatestImageTag(ctx, "flyio/postgres")
	if err != nil {
		return err
	}

	var leader *api.Machine

	// TODO - Once role/failover has been converted to http endpoints we can
	// work to orchestrate this a bit better.
	for _, machine := range machines {
		address := formatAddress(machine)

		pgclient := flypg.NewFromInstance(address, dialer)

		role, err := pgclient.NodeRole(ctx)
		if err != nil {
			return err
		}

		fmt.Fprintf(io.Out, "Machine %s is a %s\n", machine.ID, role)

		if role == "leader" {
			leader = machine
			fmt.Fprintf(io.Out, "Skipping leader(%s) for now\n", machine.ID)
			continue
		}

		err = updateMachine(ctx, cli, app.Name, imageRef, machine)
		if err != nil {
			return err
		}
	}

	if leader != nil {
		pgclient := flypg.New(app.Name, dialer)
		// fmt.Fprint(io.Out, "Triggering Failover from:  ", leader.ID)

		if err := pgclient.Failover(ctx); err != nil {
			return fmt.Errorf("failed to trigger failover %w", err)
		}
		if err := updateMachine(ctx, cli, app.Name, imageRef, leader); err != nil {
			return err
		}
	}
	return nil
}

func updateMachine(ctx context.Context, flaps *flaps.Client, appName, image string, machine *api.Machine) error {
	var io = iostreams.FromContext(ctx)

	fmt.Fprintf(io.Out, "Updating machine %s with image %s\n", machine.ID, image)
	// fmt.Fprintf(io.Out, "Current metadata: %+v", machine.Config.Metadata)

	machineConf := machine.Config
	machineConf.Image = image

	input := api.LaunchMachineInput{
		ID:      machine.ID,
		AppID:   appName,
		OrgSlug: machine.App.Organization.Slug,
		Region:  machine.Region,
		Config:  &machineConf,
	}

	resp, err := flaps.Update(ctx, input)
	if err != nil {
		return err
	}

	fmt.Fprintf(io.Out, "Response: %s", resp)

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
