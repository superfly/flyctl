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

const (
	backupVersion       = "0.0.53"
	backupConfigVersion = "0.0.54"
)

func newBackup() *cobra.Command {
	const (
		short = "Backup commands"
		long  = short + "\n"
	)

	cmd := command.New("backup", short, long, nil)
	cmd.Aliases = []string{"backups"}

	cmd.AddCommand(newBackupConfig(), newBackupCreate(), newBackupEnable(), newBackupList(), newBackupRestore())

	return cmd
}

func newBackupRestore() *cobra.Command {
	const (
		short = "Performs a WAL-based restore into a new Postgres cluster."
		long  = short + "\n"

		usage = "restore <destination-app-name>"
	)

	cmd := command.New(usage, short, long, runBackupRestore,
		command.RequireSession,
		command.RequireAppName,
	)

	cmd.Args = cobra.ExactArgs(1)

	flag.Add(
		cmd,
		flag.App(),
		flag.AppConfig(),
		flag.Detach(),
		flag.String{
			Name:        "restore-target-time",
			Description: "RFC3339-formatted timestamp up to which recovery will proceed. Example: 2021-07-16T12:34:56Z",
		},
		flag.String{
			Name:        "restore-target-name",
			Description: "ID or alias of backup to restore.",
		},
		flag.Bool{
			Name:        "restore-target-inclusive",
			Description: "Set to true to stop recovery after the specified time, or false to stop before it",
			Default:     true,
		},
		flag.String{
			Name:        "image-ref",
			Description: "Specify a non-default base image for the restored Postgres app",
		},
	)

	return cmd
}

func runBackupRestore(ctx context.Context) error {
	var (
		appName     = appconfig.NameFromContext(ctx)
		client      = flyutil.ClientFromContext(ctx)
		destAppName = flag.FirstArg(ctx)
	)

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

	// Resolve the leader
	leader, err := pickLeader(ctx, machines)
	if err != nil {
		return err
	}

	if !IsFlex(leader) {
		return fmt.Errorf("backups are only supported on Flexclusters")
	}

	enabled, err := isBackupEnabled(ctx, appName)
	if err != nil {
		return err
	}

	if !enabled {
		return fmt.Errorf("backups are not enabled. Run `fly pg backup enable -a %s` to enable them", appName)
	}

	// Ensure the the app has the required flex version.
	if err := hasRequiredFlexVersionOnMachines(appName, machines, backupVersion); err != nil {
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

	imageRef := flag.GetString(ctx, "image-ref")
	if imageRef == "" {
		imageRef = leader.FullImageRef()
	}

	// Build the input for the new cluster using the leader's configuration.
	input := &flypg.CreateClusterInput{
		AppName:                   destAppName,
		Organization:              org,
		InitialClusterSize:        1,
		ImageRef:                  imageRef,
		Region:                    leader.Region,
		Manager:                   flypg.ReplicationManager,
		Autostart:                 *leader.Config.Services[0].Autostart,
		BackupsEnabled:            false,
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
		flag.Bool{
			Name:        "immediate-checkpoint",
			Description: "Forces Postgres to perform an immediate checkpoint",
			Shorthand:   "i",
		},
	)

	return cmd
}

func runBackupCreate(ctx context.Context) error {
	var (
		appName = appconfig.NameFromContext(ctx)
	)

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

	// Ensure the backup is issued against the primary.
	leader, err := pickLeader(ctx, machines)
	if err != nil {
		return err
	}

	if !IsFlex(leader) {
		return fmt.Errorf("backups are only supported on Flexclusters")
	}

	enabled, err := isBackupEnabled(ctx, appName)
	if err != nil {
		return err
	}

	if !enabled {
		return fmt.Errorf("backups are not enabled. Run `fly pg backup enable -a %s` to enable them", appName)
	}

	if err := hasRequiredFlexVersionOnMachines(appName, machines, backupVersion); err != nil {
		return err
	}

	if !hasRequiredMemoryForBackup(*leader) {
		return fmt.Errorf("backup creation requires at least 512MB of memory. Use `fly m update %s --vm-memory 512` to scale up.", leader.ID)
	}

	cmd := "flexctl backup create"

	if flag.GetBool(ctx, "immediate-checkpoint") {
		cmd += " --immediate-checkpoint"
	}

	name := flag.GetString(ctx, "name")
	if name != "" {
		cmd += " -n " + name
	}

	return ExecOnLeader(ctx, flapsClient, cmd)
}

func newBackupEnable() *cobra.Command {
	const (
		short = "Enable backups on a Postgres cluster, creating a Tigris bucket for storage"
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

	if len(machines) == 0 {
		return fmt.Errorf("No active machines")
	}

	leader, err := pickLeader(ctx, machines)
	if err != nil {
		return err
	}

	if !IsFlex(leader) {
		return fmt.Errorf("backups are only supported on Flexclusters")
	}

	if err := hasRequiredFlexVersionOnMachines(appName, machines, backupVersion); err != nil {
		return err
	}

	if !hasRequiredMemoryForBackup(*leader) {
		return fmt.Errorf("backup creation requires at least 512MB of memory. Use `fly m update %s --vm-memory 512` to scale up.", leader.ID)
	}

	org, err := client.GetOrganizationByApp(ctx, appName)
	if err != nil {
		return err
	}

	pgInput := &flypg.CreateClusterInput{
		AppName:        appName,
		Organization:   org,
		BackupsEnabled: true,
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
	)

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

	machine := machines[0]

	if !IsFlex(machine) {
		return fmt.Errorf("backups are only supported on Flexclusters")
	}

	enabled, err := isBackupEnabled(ctx, appName)
	if err != nil {
		return err
	}

	if !enabled {
		return fmt.Errorf("backups are not enabled. Run `fly pg backup enable -a %s` to enable them", appName)
	}

	if err = hasRequiredFlexVersionOnMachines(appName, machines, backupVersion); err != nil {
		return err
	}

	return ExecOnMachine(ctx, flapsClient, machine.ID, "flexctl backup list")
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

func newBackupConfig() *cobra.Command {
	const (
		short = "Manage backup configuration"
		long  = short + "\n"
	)

	cmd := command.New("config", short, long, nil)

	cmd.AddCommand(newBackupConfigShow(), newBackupConfigUpdate())

	return cmd
}

func newBackupConfigShow() *cobra.Command {
	const (
		short = "Show backup configuration"
		long  = short + "\n"
	)

	cmd := command.New("show", short, long, runBackupConfigShow,
		command.RequireSession,
		command.RequireAppName,
	)

	flag.Add(cmd, flag.App(), flag.AppConfig())

	return cmd
}

func newBackupConfigUpdate() *cobra.Command {
	const (
		short = "Update backup configuration"
		long  = short + "\n"

		usage = "update"
	)

	cmd := command.New(usage, short, long, runBackupConfigUpdate,
		command.RequireSession,
		command.RequireAppName,
	)

	flag.Add(
		cmd,
		flag.App(),
		flag.AppConfig(),
		flag.String{
			Name:        "archive-timeout",
			Description: "Archive timeout",
		},
		flag.String{
			Name:        "recovery-window",
			Description: "Recovery window",
		},
		flag.String{
			Name:        "full-backup-frequency",
			Description: "Full backup frequency",
		},
		flag.String{
			Name:        "minimum-redundancy",
			Description: "Minimum redundancy",
		},
	)

	return cmd
}

func runBackupConfigShow(ctx context.Context) error {
	var (
		appName = appconfig.NameFromContext(ctx)
	)

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

	if !IsFlex(machines[0]) {
		return fmt.Errorf("backups are only supported on Flexclusters")
	}

	enabled, err := isBackupEnabled(ctx, appName)
	if err != nil {
		return err
	}

	if !enabled {
		return fmt.Errorf("backups are not enabled. Run `fly pg backup enable -a %s` to enable them", appName)
	}

	// Ensure the the app has the required flex version.
	if err := hasRequiredFlexVersionOnMachines(appName, machines, backupConfigVersion); err != nil {
		return err
	}

	return ExecOnLeader(ctx, flapsClient, "flexctl backup config show")
}

func runBackupConfigUpdate(ctx context.Context) error {
	var (
		appName = appconfig.NameFromContext(ctx)
	)

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

	if !IsFlex(machines[0]) {
		return fmt.Errorf("backups are only supported on Flexclusters")
	}

	// Ensure the the app has the required flex version.
	if err := hasRequiredFlexVersionOnMachines(appName, machines, backupConfigVersion); err != nil {
		return err
	}

	enabled, err := isBackupEnabled(ctx, appName)
	if err != nil {
		return err
	}

	if !enabled {
		return fmt.Errorf("backups are not enabled. Run `fly pg backup enable -a %s` to enable them", appName)
	}

	command := "flexctl backup config update"

	if flag.GetString(ctx, "archive-timeout") != "" {
		command += " --archive-timeout " + flag.GetString(ctx, "archive-timeout")
	}

	if flag.GetString(ctx, "recovery-window") != "" {
		command += " --recovery-window " + flag.GetString(ctx, "recovery-window")
	}

	if flag.GetString(ctx, "full-backup-frequency") != "" {
		command += " --full-backup-frequency " + flag.GetString(ctx, "full-backup-frequency")
	}

	if flag.GetString(ctx, "minimum-redundancy") != "" {
		command += " --minimum-redundancy " + flag.GetString(ctx, "minimum-redundancy")
	}

	return ExecOnLeader(ctx, flapsClient, command)
}
