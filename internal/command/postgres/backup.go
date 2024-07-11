package postgres

import (
	"context"
	"fmt"
	"strings"

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
	cmd.Aliases = []string{"backups"}

	cmd.AddCommand(newBackupCreate(), newBackupEnable(), newBackupList(), newBackupRestore())
	return cmd
}

func newBackupRestore() *cobra.Command {
	const (
		short = "Performs WAL-based restore into a new Postgres cluster."
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
		flag.Detach(),
		flag.String{
			Name:        "name",
			Shorthand:   "n",
			Description: "The name of the destination Postgres app",
		},
		flag.String{
			Name:        "restore-target-time",
			Description: "RFC3339-formatted timestamp up to which recovery will proceed",
		},
		flag.String{
			Name:        "restore-target-name",
			Description: "ID or alias of backup to restore",
		},
		flag.Bool{
			Name:        "restore-target-inclusive",
			Description: "Set to true to stop recovery after the specified time, or false to stop before it",
			Default:     true,
		},
	)

	return cmd
}

func runBackupRestore(ctx context.Context) error {
	var (
		appName       = appconfig.NameFromContext(ctx)
		client        = flyutil.ClientFromContext(ctx)
		targetAppName = flag.GetString(ctx, "name")
	)

	enabled, err := isBackupEnabled(ctx, appName)
	if err != nil {
		return err
	}

	if !enabled {
		return fmt.Errorf("backups are not enabled. Run `fly pg backup enable -a %s` to enable them", appName)
	}

	flapsClient, err := flapsutil.NewClientWithOptions(ctx, flaps.NewClientOpts{
		AppName: appName,
	})
	if err != nil {
		return fmt.Errorf("failed to initialize flaps client: %w", err)
	}

	machines, err := flapsClient.ListActive(ctx)
	if err != nil {
		return fmt.Errorf("list of machines could not be retrieved: %w", err)
	}

	if len(machines) == 0 {
		return fmt.Errorf("No active machines")
	}

	// Ensure the the app has the required flex version.
	if err := hasRequiredVersion(machines); err != nil {
		return err
	}

	// Resolve the leader
	leader, err := pickLeader(ctx, machines)
	if err != nil {
		return err
	}

	// TODO - Use this to create new Tigris access keys. However, if we can't yet revoke
	// access keys after the restore process completes, we should understand the implications of
	// creating potentially many access keys.
	in := &fly.MachineExecRequest{
		Cmd: "bash -c \"echo $S3_ARCHIVE_CONFIG\"",
	}

	out, err := flapsClient.Exec(ctx, leader.ID, in)
	if err != nil {
		return err
	}

	if out.StdOut == "" {
		return fmt.Errorf("S3_ARCHIVE_CONFIG is unset")
	}

	restoreSecret := strings.Trim(out.StdOut, "\n")

	// Append restore target if specified
	restoreSecret += resolveRestoreTarget(ctx)

	// Resolve organization
	org, err := client.GetOrganizationByApp(ctx, appName)
	if err != nil {
		return err
	}

	// Build the input for the new cluster using the leader's configuration.
	input := &flypg.CreateClusterInput{
		AppName:                   targetAppName,
		Organization:              org,
		InitialClusterSize:        1,
		ImageRef:                  leader.FullImageRef(),
		Region:                    leader.Region,
		Manager:                   flypg.ReplicationManager,
		Autostart:                 *leader.Config.Services[0].Autostart,
		BackupEnabled:             false,
		VolumeSize:                &leader.Config.Mounts[0].SizeGb,
		Guest:                     leader.Config.Guest,
		BarmanRemoteRestoreConfig: restoreSecret,
	}

	launcher := flypg.NewLauncher(client)
	launcher.LaunchMachinesPostgres(ctx, input, false)

	return nil
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
		flag.String{
			Name:        "name",
			Description: "Backup name",
			Shorthand:   "n",
		},
	)

	return cmd
}

func runBackupCreate(ctx context.Context) error {
	var (
		appName = appconfig.NameFromContext(ctx)
		io      = iostreams.FromContext(ctx)
	)

	enabled, err := isBackupEnabled(ctx, appName)
	if err != nil {
		return err
	}

	if !enabled {
		return fmt.Errorf("backups are not enabled. Run `fly pg backup enable -a %s` to enable them", appName)
	}

	flapsClient, err := flapsutil.NewClientWithOptions(ctx, flaps.NewClientOpts{
		AppName: appName,
	})
	if err != nil {
		return fmt.Errorf("list of machines could not be retrieved: %w", err)
	}

	machines, err := flapsClient.ListActive(ctx)
	if err != nil {
		return err
	}

	if len(machines) == 0 {
		return fmt.Errorf("No active machines")
	}

	if err := hasRequiredVersion(machines); err != nil {
		return err
	}

	// Ensure the backup is issued against the primary.
	leader, err := pickLeader(ctx, machines)
	if err != nil {
		return err
	}
	cmd := "flexctl backup create"

	// TODO - Add support for `immediate-checkpoint` flag.
	name := flag.GetString(ctx, "name")
	if name != "" {
		cmd += " -n " + name
	}

	in := &fly.MachineExecRequest{
		Cmd: cmd,
	}

	out, err := flapsClient.Exec(ctx, leader.ID, in)
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

func runBackupEnable(ctx context.Context) error {
	var (
		io      = iostreams.FromContext(ctx)
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

	// Check to see if backups are already enabled
	enabled, err := isBackupEnabled(ctx, appName)
	if err != nil {
		return err
	}

	// Short-circuit if backups are already enabled.
	if enabled {
		return fmt.Errorf("backups are already enabled")
	}

	flapsClient, err := flapsutil.NewClientWithOptions(ctx, flaps.NewClientOpts{
		AppName: appName,
	})
	if err != nil {
		return fmt.Errorf("failed to initialize flaps client: %w", err)
	}

	machines, err := flapsClient.ListActive(ctx)
	if err != nil {
		return err
	}

	if err := hasRequiredVersion(machines); err != nil {
		return err
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

	if err := flypg.CreateTigrisBucket(ctx, pgInput); err != nil {
		return fmt.Errorf("failed to create tigris bucket: %w", err)
	}

	secrets := map[string]string{
		flypg.BarmanSecretName: pgInput.BarmanSecret,
	}

	if _, err := client.SetSecrets(ctx, appName, secrets); err != nil {
		return fmt.Errorf("failed to set secrets: %w", err)
	}

	fmt.Fprintf(io.Out, "Backups enabled. Run `fly secrets deploy -a %s` to restart the cluster with the new configuration.\n", appName)
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

	enabled, err := isBackupEnabled(ctx, appName)
	if err != nil {
		return err
	}

	if !enabled {
		return fmt.Errorf("backups are not enabled. Run `fly pg backup enable -a %s` to enable them", appName)
	}

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

	err = hasRequiredVersion(machines)
	if err != nil {
		return err
	}

	machine := machines[0]

	in := &fly.MachineExecRequest{
		Cmd: "flexctl backup list",
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

func resolveRestoreTarget(ctx context.Context) string {
	target := ""
	switch {
	case flag.GetString(ctx, "restore-target-time") != "":
		target += fmt.Sprintf("?targetTime=%s", flag.GetString(ctx, "restore-target-time"))
	case flag.GetString(ctx, "restore-target-name") != "":
		target += fmt.Sprintf("?targetName=%s", flag.GetString(ctx, "restore-target-name"))
	default:
		return target
	}

	if flag.GetBool(ctx, "restore-target-inclusive") {
		target += fmt.Sprintf("&targetInclusive=%t", flag.GetBool(ctx, "restore-target-inclusive"))
	}

	return target
}

func hasRequiredVersion(machines []*fly.Machine) error {
	return hasRequiredVersionOnMachines(machines, "", "0.0.53", "")
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
