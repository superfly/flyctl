package postgres

import (
	"context"
	"encoding/base64"
	"fmt"

	"github.com/spf13/cobra"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/flypg"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/iostreams"

	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flyutil"
)

func newBackup() *cobra.Command {
	const (
		short = "Backup commands"
		long  = short + "\n"
	)

	cmd := command.New("backup", short, long, nil)

	cmd.AddCommand(newBackupCreate(), newBackupEnable(), newBackupList(), newBackupRestore())
	return cmd
}

func newBackupCreate() *cobra.Command {
	const (
		short = "Create a backup"
		long  = short + "\n"

		usage = "create"
	)

	cmd := command.New(usage, short, long, runBackupCreate,
		command.RequireSession,
		command.RequireAppName,
	)

	flag.Add(
		cmd,
		flag.App(),
		flag.AppConfig(),
	)

	return cmd
}

func runBackupCreate(ctx context.Context) error {
	return nil
}

func newBackupEnable() *cobra.Command {
	const (
		short = "Enable backups on a Postgres cluster"
		long  = short + "\n"

		usage = "enable"
	)

	cmd := command.New(usage, short, long, runBackupEnable,
		command.RequireSession,
		command.RequireAppName,
	)

	flag.Add(
		cmd,
		flag.App(),
		flag.AppConfig(),
	)

	return cmd
}

func isBackupEnabled(ctx context.Context, appName string) (bool, error) {
	var (
		client = flyutil.ClientFromContext(ctx)
	)

	secrets, err := client.GetAppSecrets(ctx, appName)
	if err != nil {
		return false, err
	}

	for _, secret := range secrets {
		if secret.Name == flypg.BarmanSecretName {
			return true, nil
		}
	}

	return false, nil
}

func runBackupEnable(ctx context.Context) error {
	var (
		appName = appconfig.NameFromContext(ctx)
		client  = flyutil.ClientFromContext(ctx)
	)

	app, err := client.GetAppCompact(ctx, appName)
	if err != nil {
		return err
	}

	if !app.IsPostgresApp() {
		return fmt.Errorf("app %s is not a postgres app", appName)
	}

	// flapsClient, err := flapsutil.NewClientWithOptions(ctx, flaps.NewClientOpts{
	// AppName: appName,
	// })
	// if err != nil {
	// 	return fmt.Errorf("list of machines could not be retrieved: %w", err)
	// }

	enabled, err := isBackupEnabled(ctx, appName)
	if err != nil {
		return err
	}

	if enabled {
		return fmt.Errorf("Backups are already enabled.")
	}

	org, err := client.GetOrganizationByApp(ctx, appName)
	if err != nil {
		return err
	}

	pgInput := &flypg.CreateClusterInput{
		AppName:       appName,
		Organization:  org,
		BackupEnabled: true,
	}

	err = flypg.CreateTigrisBucket(ctx, pgInput)
	if err != nil {
		return err
	}

	secrets := map[string]string{
		flypg.BarmanSecretName: pgInput.BarmanSecret,
	}

	if _, err := client.SetSecrets(ctx, appName, secrets); err != nil {
		return err
	}
	// TODO: Update deployment with new secrets
	return nil
}

func newBackupList() *cobra.Command {
	const (
		short = "List backups"
		long  = short + "\n"

		usage = "list"
	)

	cmd := command.New(usage, short, long, runBackupList,
		command.RequireSession,
		command.RequireAppName,
	)

	flag.Add(
		cmd,
		flag.App(),
		flag.AppConfig(),
	)

	return cmd
}

func runBackupList(ctx context.Context) error {
	var (
		appName = appconfig.NameFromContext(ctx)
		io      = iostreams.FromContext(ctx)
	)

	flapsClient, err := flapsutil.NewClientWithOptions(ctx, flaps.NewClientOpts{
		AppName: appName,
	})
	if err != nil {
		return fmt.Errorf("list of machines could not be retrieved: %w", err)
	}

	machines, err := flapsClient.List(ctx, "started")
	if err != nil {
		return err
	}

	if len(machines) == 0 {
		return fmt.Errorf("No active machines")
	}

	machine := machines[0]

	command := "barman-cloud-backup-list --cloud-provider aws-s3 --endpoint-url https://fly.storage.tigris.dev --profile barman s3://" + appName + " " + appName
	encodedCommand := base64.StdEncoding.EncodeToString([]byte(command))

	in := &fly.MachineExecRequest{
		Cmd: "su postgres bash -c \"$(echo " + encodedCommand + " | base64 --decode)\"",
	}

	out, err := flapsClient.Exec(ctx, machine.ID, in)
	if err != nil {
		return err
	}

	if out.ExitCode != 0 {
		fmt.Fprintf(io.Out, "Exit code: %d\n", out.ExitCode)
	}

	if out.StdOut != "" {
		fmt.Fprint(io.Out, out.StdOut)
	}
	if out.StdErr != "" {
		fmt.Fprint(io.ErrOut, out.StdErr)
	}

	return nil
}

func newBackupRestore() *cobra.Command {
	const (
		short = "Restore a backup"
		long  = short + "\n"

		usage = "restore"
	)

	cmd := command.New(usage, short, long, runBackupRestore,
		command.RequireSession,
		command.RequireAppName,
	)

	flag.Add(
		cmd,
		flag.App(),
		flag.AppConfig(),
	)

	return cmd
}

func runBackupRestore(ctx context.Context) error {
	return nil
}
