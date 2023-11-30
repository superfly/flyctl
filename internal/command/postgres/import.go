package postgres

import (
	"context"
	"fmt"
	"os"

	"github.com/docker/docker/pkg/ioutils"
	"github.com/mattn/go-colorable"
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/agent"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/command/apps"
	"github.com/superfly/flyctl/internal/command/ssh"
	"github.com/superfly/flyctl/internal/flag"
	mach "github.com/superfly/flyctl/internal/machine"
	"github.com/superfly/flyctl/internal/prompt"
)

func newImport() *cobra.Command {
	const (
		short = "Imports database from a specified Postgres URI"
		long  = short + "\n"
		usage = "import <source-uri>"
	)

	cmd := command.New(usage, short, long, runImport,
		command.RequireSession,
		command.RequireAppName,
	)

	cmd.Args = cobra.ExactArgs(1)

	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
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
			Default:     false,
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
		client  = client.FromContext(ctx).API()
		appName = appconfig.NameFromContext(ctx)

		sourceURI = flag.FirstArg(ctx)
		machSize  = flag.GetString(ctx, "vm-size")
		imageRef  = flag.GetString(ctx, "image")
	)

	// pre-fetch platform regions for later use
	prompt.PlatformRegions(ctx)

	// Resolve target app
	app, err := client.GetAppCompact(ctx, appName)
	if err != nil {
		return fmt.Errorf("failed to resolve app: %w", err)
	}

	if app.PlatformVersion != "machines" {
		return fmt.Errorf("This feature is only available on our Machines platform")
	}

	if !app.IsPostgresApp() {
		return fmt.Errorf("The target app must be a Postgres app")
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
		Restart: &api.MachineRestart{
			Policy: api.MachineRestartPolicyNo,
		},
		AutoDestroy: true,
	}

	// If a custom migration image is not specified, resolve latest managed image.
	if imageRef == "" {
		imageRef, err = client.GetLatestImageTag(ctx, "flyio/postgres-importer", nil)
		if err != nil {
			return err
		}
	}
	machineConfig.Image = imageRef

	ephemeralInput := &mach.EphemeralInput{
		LaunchInput: api.LaunchMachineInput{
			Region: region.Code,
			Config: machineConfig,
		},
		What: "to run the import process",
	}

	// Create ephemeral machine
	machine, cleanup, err := mach.LaunchEphemeral(ctx, ephemeralInput)
	if err != nil {
		return err
	}
	defer cleanup()

	// Initiate migration process
	err = ssh.SSHConnect(&ssh.SSHParams{
		Ctx:      ctx,
		Org:      app.Organization,
		Dialer:   agent.DialerFromContext(ctx),
		App:      app.Name,
		Username: ssh.DefaultSshUsername,
		Cmd:      resolveImportCommand(ctx),
		Stdin:    os.Stdin,
		Stdout:   ioutils.NewWriteCloserWrapper(colorable.NewColorableStdout(), func() error { return nil }),
		Stderr:   ioutils.NewWriteCloserWrapper(colorable.NewColorableStderr(), func() error { return nil }),
	}, machine.PrivateIP)
	if err != nil {
		return fmt.Errorf("failed to run ssh: %s", err)
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

	return fmt.Sprintf(
		"migrate -no-owner=%v -create=%v -clean=%v -data-only=%v",
		noOwner, create, clean, dataOnly,
	)
}
