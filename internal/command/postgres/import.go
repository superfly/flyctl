package postgres

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/docker/docker/pkg/ioutils"
	"github.com/mattn/go-colorable"
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/agent"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/flaps"
	"github.com/superfly/flyctl/internal/app"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/command/apps"
	"github.com/superfly/flyctl/internal/command/ssh"
	"github.com/superfly/flyctl/internal/flag"
	mach "github.com/superfly/flyctl/internal/machine"
	"github.com/superfly/flyctl/internal/prompt"
	"github.com/superfly/flyctl/iostreams"
)

func newImport() *cobra.Command {
	const (
		short = "Imports database from a specified Postgres URI"
		long  = short + "\n"
		usage = "import"
	)

	cmd := command.New(usage, short, long, runImport,
		command.RequireSession,
		command.RequireAppName,
	)

	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
		flag.String{
			Name:        "source-uri",
			Description: "The target postgres uri. This should target the individual database you wish to migrate.",
		},
		flag.String{
			Name:        "image",
			Description: "Path to public image containing custom migration process",
		},
		flag.String{
			Name:        "vm-size",
			Description: "the size of the VM",
		},
		flag.String{
			Name:        "region",
			Description: "Region to provision migration machine",
		},
		flag.Bool{
			Name:        "no-owner",
			Description: "Do not set ownership of objects to match the original database. Makes dump restorable by any user.",
			Default:     true,
		},
		flag.Bool{
			Name:        "create",
			Description: "Begin by creating the database itself and reconnecting to it. If --clean is also specified, the script drops and recreates the target database before reconnecting to it.",
			Default:     true,
		},
		flag.Bool{
			Name:        "clean",
			Description: "Drop database objects prior to creating them.",
			Default:     true,
		},
		flag.Bool{
			Name:        "data-only",
			Description: "Dump only the data, not the schema (data definitions).",
		},
	)

	return cmd
}

func runImport(ctx context.Context) error {
	var (
		io      = iostreams.FromContext(ctx)
		client  = client.FromContext(ctx).API()
		appName = app.NameFromContext(ctx)

		sourceURI = flag.GetString(ctx, "source-uri")
		machSize  = flag.GetString(ctx, "vm-size")
		imageRef  = flag.GetString(ctx, "image")
	)

	// Resolve target app
	app, err := client.GetAppCompact(ctx, appName)
	if err != nil {
		return fmt.Errorf("failed to resolve app: %w", err)
	}

	// Resolve region
	region, err := prompt.Region(ctx, !app.Organization.PaidPlan, prompt.RegionParams{
		Message: "Choose a region to deploy the migration machine:",
	})
	if err != nil {
		return fmt.Errorf("failed to resolve region: %s", err)
	}

	// Resolve vm-size
	vmSize, err := resolveVMSize(ctx, machSize)
	if err != nil {
		return err
	}

	// Set sourceURI as a secret
	_, err = client.SetSecrets(ctx, app.Name, map[string]string{
		"SOURCE_DATABASE_URI": sourceURI,
	})
	if err != nil {
		return fmt.Errorf("failed to set secrets: %s", err)
	}

	ctx, err = apps.BuildContext(ctx, app)
	if err != nil {
		return fmt.Errorf("failed to build context: %s", err)
	}

	flapsClient := flaps.FromContext(ctx)

	machineConfig := &api.MachineConfig{
		Env: map[string]string{
			"POSTGRES_PASSWORD": "pass",
		},
		Guest: &api.MachineGuest{
			CPUKind:  vmSize.CPUClass,
			CPUs:     int(vmSize.CPUCores),
			MemoryMB: vmSize.MemoryMB,
		},
		DNS: &api.DNSConfig{
			SkipRegistration: true,
		},
		Restart: api.MachineRestart{
			Policy: api.MachineRestartPolicyNo,
		},
	}

	// If a custom migration image is not specified, resolve latest managed image.
	if imageRef == "" {
		imageRef, err = client.GetLatestImageTag(ctx, "flyio/postgres-importer", nil)
		if err != nil {
			return err
		}
	}
	machineConfig.Image = imageRef

	launchInput := api.LaunchMachineInput{
		AppID:   app.ID,
		OrgSlug: app.Organization.ID,
		Region:  region.Code,
		Config:  machineConfig,
	}

	// Create emphemeral machine
	machine, err := flapsClient.Launch(ctx, launchInput)
	if err != nil {
		return err
	}

	fmt.Fprintf(io.Out, "Waiting for machine %s to start...\n", machine.ID)
	err = mach.WaitForStartOrStop(ctx, machine, "start", time.Minute*1)
	if err != nil {
		return err
	}

	// Initiate migration process
	err = ssh.SSHConnect(&ssh.SSHParams{
		Ctx:    ctx,
		Org:    app.Organization,
		Dialer: agent.DialerFromContext(ctx),
		App:    app.Name,
		Cmd:    resolveImportCommand(ctx),
		Stdin:  os.Stdin,
		Stdout: ioutils.NewWriteCloserWrapper(colorable.NewColorableStdout(), func() error { return nil }),
		Stderr: ioutils.NewWriteCloserWrapper(colorable.NewColorableStderr(), func() error { return nil }),
	}, machine.PrivateIP)
	if err != nil {
		return fmt.Errorf("failed to run ssh: %s", err)
	}

	// Stop Machine
	if err := flapsClient.Stop(ctx, api.StopMachineInput{ID: machine.ID}); err != nil {
		return err
	}

	fmt.Fprintf(io.Out, "Waiting for machine %s to stop...\n", machine.ID)
	err = mach.WaitForStartOrStop(ctx, machine, "stop", time.Minute*1)
	if err != nil {
		return fmt.Errorf("failed waiting for machine %s to stop: %s", machine.ID, err)
	}

	// Destroy machine
	fmt.Fprintf(io.Out, "%s has been destroyed\n", machine.ID)
	if err := flapsClient.Destroy(ctx, api.RemoveMachineInput{ID: machine.ID, AppID: app.ID}); err != nil {
		return fmt.Errorf("failed to destroy machine %s: %s", machine.ID, err)
	}

	// Unset secret
	_, err = client.UnsetSecrets(ctx, app.Name, []string{"SOURCE_DATABASE_URI"})
	if err != nil {
		return fmt.Errorf("failed to set secrets: %s", err)
	}

	return nil
}

func resolveImportCommand(ctx context.Context) string {
	var (
		noOwner  = flag.GetBool(ctx, "no-owner")
		create   = flag.GetBool(ctx, "create")
		clean    = flag.GetBool(ctx, "clean")
		dataOnly = flag.GetBool(ctx, "data-only")
	)

	importCmd := "migrate "
	if noOwner {
		importCmd = importCmd + " -no-owner"
	}
	if clean {
		importCmd = importCmd + " -clean"
	}
	if create {
		importCmd = importCmd + " -create"
	}
	if dataOnly {
		importCmd = importCmd + " -data-only"
	}

	return importCmd
}
