package postgres

import (
	"context"
	"fmt"
	"strings"
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
	"github.com/superfly/flyctl/internal/prompt"
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
		flag.Bool{
			Name:        "auto-confirm",
			Description: "Will automatically confirm changes without an interactive prompt.",
		},
	)

	return
}

func runUpdate(ctx context.Context) error {
	appName := app.NameFromContext(ctx)
	client := client.FromContext(ctx).API()
	io := iostreams.FromContext(ctx)

	// only target machines running a valid repository
	const validRepository = "flyio/postgres"

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

	machineList, err := flapsClient.List(ctx, "started")
	if err != nil {
		return fmt.Errorf("machines could not be retrieved %w", err)
	}

	// Tracks latest eligible version
	var latest *api.ImageVersion

	// Filter out machines that are not eligible for updates
	var machines []*api.Machine
	for _, machine := range machineList {
		// Skip machines that are not running our internal images.
		if machine.ImageRef.Repository != validRepository {
			continue
		}

		image := fmt.Sprintf("%s:%s", machine.ImageRef.Repository, machine.ImageRef.Tag)
		latestImage, err := client.GetLatestImageDetails(ctx, image)
		if err != nil {
			return fmt.Errorf("unable to fetch latest image details for %s: %w", image, err)
		}

		if latest == nil {
			latest = latestImage
		}

		// Abort if we detect a postgres machine running a different major version.
		if latest.Tag != latestImage.Tag {
			return fmt.Errorf("major version mismatch detected")
		}

		// Exclude machines that are already running the latest version
		if machine.ImageRef.Tag == latest.Tag && machine.ImageVersion() == latest.Version {
			continue
		}

		machines = append(machines, machine)
	}

	if len(machines) == 0 {
		fmt.Fprintf(io.Out, "No updates available...\n")
		return nil
	}

	// Confirmation prompt
	if !flag.GetBool(ctx, "auto-confirm") {
		msgs := []string{"The following machine(s) will be updated:\n"}
		for _, machine := range machines {
			latestStr := fmt.Sprintf("%s:%s (%s)", latest.Repository, latest.Tag, latest.Version)
			msg := fmt.Sprintf("Machine %q %s -> %s\n", machine.ID, machine.ImageRefWithVersion(), latestStr)
			msgs = append(msgs, msg)
		}
		msgs = append(msgs, "\nPerform the specified update(s)?")

		switch confirmed, err := prompt.Confirmf(ctx, strings.Join(msgs, "")); {
		case err == nil:
			if !confirmed {
				return nil
			}
		case prompt.IsNonInteractive(err):
			return prompt.NonInteractiveError("auto-confirm flag must be specified when not running interactively")
		default:
			return err
		}
	}

	// TODO - Verify cluster health before performing update.

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
	if len(replicas) > 0 {
		fmt.Fprintf(io.Out, "Updating replica(s)\n")

		for _, replica := range replicas {
			image := fmt.Sprintf("%s:%s", latest.Repository, latest.Tag)

			fmt.Fprintf(io.Out, "  Updating machine %s with image %s %s\n", replica.ID, image, latest.Version)
			if err := updateMachine(ctx, app, replica, image); err != nil {
				return fmt.Errorf("can't update %s: %w", replica.ID, err)
			}
		}
	}

	// Update leader
	if leader != nil {
		// Only perform failover if there's potentially eligible replicas
		if len(machines) > 1 {
			pgclient := flypg.New(app.Name, dialer)

			fmt.Fprintf(io.Out, "Performing failover\n")
			if err := pgclient.Failover(ctx); err != nil {
				return fmt.Errorf("failed to trigger failover %w", err)
			}
		}

		fmt.Fprintf(io.Out, "Updating replica\n")
		image := fmt.Sprintf("%s:%s", latest.Repository, latest.Tag)

		fmt.Fprintf(io.Out, "  Updating machine %s with image %s %s\n", leader.ID, image, latest.Version)
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

	if err := machines.WaitForStartOrStop(ctx, flaps, updated, "start", time.Minute*5); err != nil {
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
