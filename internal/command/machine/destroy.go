package machine

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/flyutil"
	mach "github.com/superfly/flyctl/internal/machine"
	"github.com/superfly/flyctl/internal/prompt"
	"github.com/superfly/flyctl/iostreams"
)

func newDestroy() *cobra.Command {
	const (
		short = "Destroy Fly machines"
		long  = `Destroy one or more Fly machines.
This command requires a machine to be in a stopped or suspended state unless the force flag is used.
`
		usage = "destroy [flags] ID ID ..."
	)

	cmd := command.New(usage, short, long, runMachineDestroy,
		command.RequireSession,
		command.LoadAppNameIfPresent,
	)

	cmd.Aliases = []string{"remove", "rm"}

	flag.Add(
		cmd,
		flag.App(),
		flag.AppConfig(),
		selectFlag,
		flag.Bool{
			Name:        "force",
			Shorthand:   "f",
			Description: "force kill machine regardless of current state",
		},
		flag.String{
			Name:        "image",
			Shorthand:   "i",
			Description: "remove all machines with the specified image hash",
		},
	)

	cmd.Args = cobra.ArbitraryArgs

	return cmd
}

func runMachineDestroy(ctx context.Context) (err error) {
	ctx, err = buildContextFromAppName(ctx, appconfig.NameFromContext(ctx))
	if err != nil {
		return err
	}

	var machinesToBeDeleted []*fly.Machine
	image := strings.TrimSpace(flag.GetString(ctx, "image"))

	switch {
	case image != "":
		machines, err := flapsutil.ClientFromContext(ctx).ListActive(ctx)
		if err != nil {
			return err
		}

		var ids []string
		for _, machine := range machines {
			if machine.ImageRefWithVersion() == image {
				machinesToBeDeleted = append(machinesToBeDeleted, machine)
				ids = append(ids, machine.ID)
			}
		}

		confirmed, err := prompt.Confirm(ctx,
			fmt.Sprintf("%d Machines (%s) will be destroyed, continue?",
				len(machinesToBeDeleted),
				strings.Join(ids, ","),
			))
		if err != nil {
			return err
		}
		if !confirmed {
			return nil
		}

	case len(flag.Args(ctx)) == 0:
		machine, newCtx, err := selectOneMachine(ctx, "", "", false)
		if err != nil {
			return err
		}
		ctx = newCtx
		machinesToBeDeleted = append(machinesToBeDeleted, machine)

	default:
		machines, newCtx, err := selectManyMachines(ctx, flag.Args(ctx))
		if err != nil {
			return err
		}
		ctx = newCtx
		machinesToBeDeleted = append(machinesToBeDeleted, machines...)
	}

	if len(machinesToBeDeleted) == 0 {
		fmt.Fprint(iostreams.FromContext(ctx).Out, "No machine to destroy, exiting\n")
		return nil
	}

	machines, release, err := mach.AcquireLeases(ctx, machinesToBeDeleted)
	defer release()
	if err != nil {
		return err
	}

	for _, machine := range machines {
		err = singleDestroyRun(ctx, machine)
		if err != nil {
			return err
		}
	}

	return nil
}

func singleDestroyRun(ctx context.Context, machine *fly.Machine) error {
	var (
		out   = iostreams.FromContext(ctx).Out
		force = flag.GetBool(ctx, "force")
	)

	appName := appconfig.NameFromContext(ctx)

	// This is used for the deletion hook below.
	client := flyutil.ClientFromContext(ctx)
	app, err := client.GetAppCompact(ctx, appName)
	if err != nil {
		return fmt.Errorf("could not get app '%s': %w", appName, err)
	}

	err = Destroy(ctx, app, machine, force)
	if err != nil {
		return err
	}

	fmt.Fprintf(out, "%s has been destroyed\n", machine.ID)

	return nil
}

func Destroy(ctx context.Context, app *fly.AppCompact, machine *fly.Machine, force bool) error {
	var (
		out         = iostreams.FromContext(ctx).Out
		flapsClient = flapsutil.ClientFromContext(ctx)

		input = fly.RemoveMachineInput{
			ID:   machine.ID,
			Kill: force,
		}
	)

	switch machine.State {
	case "stopped", "suspended":
		break
	case "destroyed":
		return fmt.Errorf("machine %s has already been destroyed", machine.ID)
	case "started":
		if !force {
			return fmt.Errorf("machine %s currently started, either stop first or use --force flag", machine.ID)
		}
	default:
		if !force {
			return fmt.Errorf("machine %s is in a %s state and cannot be destroyed since it is not stopped or suspended, either stop first or use --force flag", machine.ID, machine.State)
		}
	}
	fmt.Fprintf(out, "machine %s was found and is currently in %s state, attempting to destroy...\n", machine.ID, machine.State)

	err := flapsClient.Destroy(ctx, input, machine.LeaseNonce)
	if err != nil {
		if err := rewriteMachineNotFoundErrors(ctx, err, machine.ID); err != nil {
			return err
		}
		return fmt.Errorf("could not destroy machine %s: %w", machine.ID, err)
	}

	// Best effort post-deletion hook.
	runOnDeletionHook(ctx, app, machine)

	return nil
}
