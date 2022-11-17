package image

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/agent"
	"github.com/superfly/flyctl/flaps"
	"github.com/superfly/flyctl/flypg"
	"github.com/superfly/flyctl/iostreams"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/internal/app"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/command/apps"
	"github.com/superfly/flyctl/internal/flag"
	mach "github.com/superfly/flyctl/internal/machine"
	"github.com/superfly/flyctl/internal/prompt"
)

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
		flag.String{
			Name:        "image",
			Description: "Target a specific image",
		},
		flag.Bool{
			Name:        "auto-confirm",
			Description: "Auto-confirm changes",
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

	ctx, err = apps.BuildContext(ctx, app)
	if err != nil {
		return err
	}

	switch app.PlatformVersion {
	case "nomad":
		return updateImageForNomad(ctx)
	case "machines":
		if app.IsPostgresApp() {
			return updatePostgresOnMachines(ctx, app)
		}
		return updateImageForMachines(ctx, app)
	}
	return
}

func updateImageForMachines(ctx context.Context, app *api.AppCompact) error {
	var (
		io          = iostreams.FromContext(ctx)
		flapsClient = flaps.FromContext(ctx)
		colorize    = io.ColorScheme()

		autoConfirm = flag.GetBool(ctx, "auto-confirm")
	)

	machines, err := mach.AcquireLeases(ctx)
	if err != nil {
		return err
	}
	for _, machine := range machines {
		defer flapsClient.ReleaseLease(ctx, machine.ID, machine.LeaseNonce)
	}

	eligible := map[*api.Machine]api.MachineConfig{}

	// Loop through machines and compare/confirm changes.
	for _, machine := range machines {
		machineConf, err := mach.CloneConfig(*machine.Config)
		if err != nil {
			return err
		}

		image, err := resolveImage(ctx, *machine)
		if err != nil {
			return err
		}

		machineConf.Image = image

		if !autoConfirm {
			diff := mach.ConfigCompare(ctx, *machine.Config, *machineConf)
			fmt.Fprintf(io.Out, "You are about to apply the following changes to machine %s.\n", colorize.Bold(machine.ID))
			fmt.Fprintf(io.Out, "%s\n", diff)

			const msg = "Apply changes?"

			switch confirmed, err := prompt.Confirmf(ctx, msg); {
			case err == nil:
				if !confirmed {
					continue
				}
			case prompt.IsNonInteractive(err):
				return prompt.NonInteractiveError("auto-confirm flag must be specified when not running interactively")
			default:
				return err
			}
		}

		eligible[machine] = *machineConf
	}

	for machine, machineConf := range eligible {
		input := &api.LaunchMachineInput{
			ID:      machine.ID,
			AppID:   app.Name,
			OrgSlug: app.Organization.Slug,
			Region:  machine.Region,
			Config:  &machineConf,
		}
		if err := mach.Update(ctx, machine, input, autoConfirm); err != nil {
			return err
		}
	}

	fmt.Fprintln(io.Out, "Machines successfully updated")

	return nil
}

type Member struct {
	Machine      *api.Machine
	TargetConfig api.MachineConfig
}

func updatePostgresOnMachines(ctx context.Context, app *api.AppCompact) (err error) {
	var (
		io          = iostreams.FromContext(ctx)
		colorize    = io.ColorScheme()
		flapsClient = flaps.FromContext(ctx)
		autoConfirm = flag.GetBool(ctx, "auto-confirm")
	)

	// Acquire leases
	machines, err := mach.AcquireLeases(ctx)
	if err != nil {
		return err
	}
	for _, machine := range machines {
		defer flapsClient.ReleaseLease(ctx, machine.ID, machine.LeaseNonce)
	}

	// Identify target images
	members := map[string][]Member{}

	for _, machine := range machines {
		machineConf, err := mach.CloneConfig(*machine.Config)
		if err != nil {
			return err
		}

		image, err := resolveImage(ctx, *machine)
		if err != nil {
			return err
		}

		machineConf.Image = image

		if !autoConfirm {
			diff := mach.ConfigCompare(ctx, *machine.Config, *machineConf)
			if diff == "" {
				continue
			}
			fmt.Fprintf(io.Out, "Configuration changes to be applied to machine: %s.\n", colorize.Bold(machine.ID))
			fmt.Fprintf(io.Out, "%s\n", diff)

			const msg = "Apply changes?"

			switch confirmed, err := prompt.Confirmf(ctx, msg); {
			case err == nil:
				if !confirmed {
					continue
				}
			case prompt.IsNonInteractive(err):
				return prompt.NonInteractiveError("auto-confirm flag must be specified when not running interactively")
			default:
				return err
			}
		}

		role := machineRole(machine)
		member := Member{Machine: machine, TargetConfig: *machineConf}
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

	for _, member := range members["replica"] {
		machine := member.Machine
		input := &api.LaunchMachineInput{
			ID:      machine.ID,
			AppID:   app.Name,
			OrgSlug: app.Organization.Slug,
			Region:  machine.Region,
			Config:  &member.TargetConfig,
		}
		if err := mach.Update(ctx, machine, input, false); err != nil {
			return err
		}
	}

	if len(members["leader"]) == 0 {

	} else if len(members["leader"]) > 1 {

	} else {
		leader := members["leader"][0]
		machine := leader.Machine

		dialer := agent.DialerFromContext(ctx)
		pgclient := flypg.NewFromInstance(machine.PrivateIP, dialer)
		fmt.Fprintf(io.Out, "Attempting to failover %s\n", colorize.Bold(machine.ID))

		if err := pgclient.Failover(ctx); err != nil {
			fmt.Fprintln(io.Out, colorize.Red(fmt.Sprintf("failed to perform failover: %s", err.Error())))
		}

		input := &api.LaunchMachineInput{
			ID:      machine.ID,
			AppID:   app.Name,
			OrgSlug: app.Organization.Slug,
			Region:  machine.Region,
			Config:  &leader.TargetConfig,
		}
		if err := mach.Update(ctx, machine, input, false); err != nil {
			return err
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
		if err != nil && strings.Contains(err.Error(), "Unknown repository") {
			// do something
		}

		if latestImage != nil {
			image = fmt.Sprintf("%s/%s", latestImage.Registry, latestImage.FullImageRef())
		}

		if image == "" {
			image = machine.Config.Image
		}
	}

	return image, nil
}
