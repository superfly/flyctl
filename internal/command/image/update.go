package image

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/agent"
	"github.com/superfly/flyctl/flaps"
	"github.com/superfly/flyctl/flypg"
	"github.com/superfly/flyctl/iostreams"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/internal/app"
	"github.com/superfly/flyctl/internal/command"
	machines "github.com/superfly/flyctl/internal/command/machine"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/prompt"
	"github.com/superfly/flyctl/internal/render"
	"github.com/superfly/flyctl/internal/watch"
)

// only target machines running a valid repository
const validRepository = "flyio/postgres"

func newUpdate() *cobra.Command {
	const (
		long = `This will update the application's image to the latest available version.
The update will perform a rolling restart against each VM, which may result in a brief service disruption.`

		short = "Updates the app's image to the latest available version. (Fly Postgres only)"

		usage = "update"
	)

	cmd := command.New(usage, short, long, runUpdate,
		command.RequireSession,
		command.RequireAppName,
	)

	cmd.Args = cobra.NoArgs

	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
		flag.Yes(),
		flag.String{
			Name:        "strategy",
			Description: "Deployment strategy",
		},
		flag.Bool{
			Name:        "detach",
			Description: "Return immediately instead of monitoring update progress",
		},
		flag.Bool{
			Name:        "auto-confirm",
			Description: "Will automatically confirm changes without an interactive prompt.",
		},
	)

	return cmd
}

func runUpdate(ctx context.Context) (err error) {
	var (
		appName = app.NameFromContext(ctx)
		client  = client.FromContext(ctx).API()
	)

	app, err := client.GetAppCompact(ctx, appName)
	if err != nil {
		return fmt.Errorf("get app: %w", err)
	}

	switch app.PlatformVersion {
	case "nomad":
		return updateImageForNomad(ctx)
	case "machines":
		return updateImageForMachines(ctx)
	}
	return
}

func updateImageForNomad(ctx context.Context) error {
	var (
		client  = client.FromContext(ctx).API()
		io      = iostreams.FromContext(ctx)
		appName = app.NameFromContext(ctx)
	)

	app, err := client.GetImageInfo(ctx, appName)
	if err != nil {
		return fmt.Errorf("failed to get image info: %w", err)
	}

	if !app.ImageVersionTrackingEnabled {
		return errors.New("image is not eligible for automated image updates")
	}

	if !app.ImageUpgradeAvailable {
		return errors.New("image is already running the latest image")
	}

	cI := app.ImageDetails
	lI := app.LatestImageDetails

	current := fmt.Sprintf("%s:%s", cI.Repository, cI.Tag)
	target := fmt.Sprintf("%s:%s", lI.Repository, lI.Tag)

	if cI.Version != "" {
		current = fmt.Sprintf("%s %s", current, cI.Version)
	}

	if lI.Version != "" {
		target = fmt.Sprintf("%s %s", target, lI.Version)
	}

	if !flag.GetYes(ctx) {
		switch confirmed, err := prompt.Confirmf(ctx, "Update `%s` from %s to %s?", appName, current, target); {
		case err == nil:
			if !confirmed {
				return nil
			}
		case prompt.IsNonInteractive(err):
			return prompt.NonInteractiveError("yes flag must be specified when not running interactively")
		default:
			return err
		}
	}

	input := api.DeployImageInput{
		AppID:    appName,
		Image:    fmt.Sprintf("%s:%s", lI.Repository, lI.Tag),
		Strategy: api.StringPointer("ROLLING"),
	}

	// Set the deployment strategy
	if val := flag.GetString(ctx, "strategy"); val != "" {
		input.Strategy = api.StringPointer(strings.ReplaceAll(strings.ToUpper(val), "-", "_"))
	}

	release, releaseCommand, err := client.DeployImage(ctx, input)
	if err != nil {
		return err
	}

	fmt.Fprintf(io.Out, "Release v%d created\n", release.Version)
	if releaseCommand != nil {
		fmt.Fprintln(io.Out, "Release command detected: this new release will not be available until the command succeeds.")
	}

	fmt.Fprintln(io.Out)

	tb := render.NewTextBlock(ctx)

	tb.Detail("You can detach the terminal anytime without stopping the update")

	if releaseCommand != nil {
		// TODO: don't use text block here
		tb := render.NewTextBlock(ctx, fmt.Sprintf("Release command detected: %s\n", releaseCommand.Command))
		tb.Done("This release will not be available until the release command succeeds.")

		if err := watch.ReleaseCommand(ctx, releaseCommand.ID); err != nil {
			return err
		}

		release, err = client.GetAppRelease(ctx, appName, release.ID)
		if err != nil {
			return err
		}
	}
	return watch.Deployment(ctx, appName, release.EvaluationID)
}

func updateImageForMachines(ctx context.Context) (err error) {
	var (
		io      = iostreams.FromContext(ctx)
		appName = app.NameFromContext(ctx)
		client  = client.FromContext(ctx).API()
	)

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
	ctx = agent.DialerWithContext(ctx, dialer)

	flapsClient, err := flaps.New(ctx, app)
	if err != nil {
		return fmt.Errorf("list of machines could not be retrieved: %w", err)
	}
	ctx = flaps.NewContext(ctx, flapsClient)

	candidates, err := flapsClient.List(ctx, "started")
	if err != nil {
		return fmt.Errorf("machines could not be retrieved %w", err)
	}

	// Tracks latest eligible version
	var latest *api.ImageVersion

	// Filter out eligible that are not eligible for updates
	var eligible []*api.Machine

	for _, machine := range candidates {
		// Skip machines that are not running our internal images.
		if machine.ImageRef.Repository != validRepository && machine.ImageVersionTrackingEnabled() {
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

		eligible = append(eligible, machine)
	}

	if len(candidates) == 0 {
		fmt.Fprintf(io.Out, "No updates available...\n")
		return nil
	}

	// Confirmation prompt
	if !flag.GetBool(ctx, "auto-confirm") {
		msgs := []string{"The following machine(s) will be updated:\n"}
		for _, machine := range candidates {
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

	// Acquire leases
	fmt.Fprintf(io.Out, "Attempting to acquire lease(s)\n")
	for _, machine := range candidates {
		lease, err := flapsClient.GetLease(ctx, machine.ID, api.IntPointer(40))
		if err != nil {
			return fmt.Errorf("failed to obtain lease: %w", err)
		}
		machine.LeaseNonce = lease.Data.Nonce

		// Ensure lease is released on return
		defer releaseLease(ctx, machine)

		fmt.Fprintf(io.Out, "  Machine %s: %s\n", machine.ID, lease.Status)
	}

	if app.IsPostgresApp() {
		return updatePostgres(ctx, app, candidates, latest)
	}

	// Update replicas
	if len(eligible) > 0 {
		fmt.Fprintf(io.Out, "Updating machines\n")

		for _, machine := range eligible {
			image := fmt.Sprintf("%s:%s", latest.Repository, latest.Tag)

			fmt.Fprintf(io.Out, "  Updating machine %s with image %s %s\n", machine.ID, image, latest.Version)
			if err := updateMachine(ctx, app, machine, image); err != nil {
				return fmt.Errorf("can't update %s: %w", machine.ID, err)
			}

			// wait for health checks to pass
			if err := watch.MachinesChecks(ctx, []*api.Machine{machine}); err != nil {
				return fmt.Errorf("failed to wait for health checks to pass: %w", err)
			}
		}
	}

	return
}

func updateMachine(ctx context.Context, app *api.AppCompact, machine *api.Machine, image string) error {
	var flapsClient = flaps.FromContext(ctx)

	machineConf := machine.Config
	machineConf.Image = image

	input := api.LaunchMachineInput{
		ID:      machine.ID,
		AppID:   app.Name,
		OrgSlug: app.Organization.Slug,
		Region:  machine.Region,
		Config:  machineConf,
	}

	updated, err := flapsClient.Update(ctx, input, machine.LeaseNonce)
	if err != nil {
		return err
	}

	if err := machines.WaitForStartOrStop(ctx, updated, "start", time.Minute*5); err != nil {
		return err
	}

	return nil
}

func updatePostgres(ctx context.Context, app *api.AppCompact, machines []*api.Machine, latest *api.ImageVersion) error {
	var (
		io     = iostreams.FromContext(ctx)
		dialer = agent.DialerFromContext(ctx)
	)

	// Resolve cluster roles
	fmt.Fprintf(io.Out, "Identifying cluster roles\n")
	var (
		leader   *api.Machine
		replicas []*api.Machine
	)

	for _, machine := range machines {
		address := fmt.Sprintf("[%s]", machine.PrivateIP)

		pgclient := flypg.NewFromInstance(address, dialer)

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

	// Update replicas
	if len(replicas) > 0 {
		fmt.Fprintf(io.Out, "Updating machines\n")

		for _, machine := range replicas {
			image := fmt.Sprintf("%s:%s", latest.Repository, latest.Tag)

			fmt.Fprintf(io.Out, "  Updating machine %s with image %s %s\n", machine.ID, image, latest.Version)
			if err := updateMachine(ctx, app, machine, image); err != nil {
				return fmt.Errorf("can't update %s: %w", machine.ID, err)
			}

			// wait for health checks to pass
			if err := watch.MachinesChecks(ctx, []*api.Machine{machine}); err != nil {
				return fmt.Errorf("failed to wait for health checks to pass: %w", err)
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

		// wait for health checks to pass
		if err := watch.MachinesChecks(ctx, []*api.Machine{leader}); err != nil {
			return fmt.Errorf("failed to wait for health checks to pass: %w", err)
		}
	}

	return nil
}

func releaseLease(ctx context.Context, machine *api.Machine) error {
	var client = flaps.FromContext(ctx)

	if err := client.ReleaseLease(ctx, machine.ID, machine.LeaseNonce); err != nil {
		return fmt.Errorf("failed to release lease: %w", err)
	}

	return nil
}
