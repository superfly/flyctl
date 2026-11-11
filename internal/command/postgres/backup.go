package postgres

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/flypg"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/appsecrets"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/uiexutil"
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
		short = "Manage Fly Postgres backups"
		long  = `Manage WAL-based backups of a Fly Postgres cluster, stored in a Tigris
bucket. Backups work only on clusters managed by repmgr (Flex), the
default for ` + "`fly postgres create`" + `. Turn them on with ` + "`fly postgres backup enable`" + `,
then use ` + "`fly postgres backup list`" + ` and ` + "`fly postgres backup restore`" + `.`
	)

	cmd := command.New("backup", short, long, nil)
	cmd.Aliases = []string{"backups"}

	cmd.AddCommand(newBackupConfig(), newBackupCreate(), newBackupEnable(), newBackupList(), newBackupRestore())

	return cmd
}

func newBackupRestore() *cobra.Command {
	const (
		short = "Restore a Fly Postgres cluster into a new cluster"
		long  = `Restore a Fly Postgres cluster's backups into a new single-machine
cluster. The source cluster comes from --app or fly.toml and is not
changed; the argument is the name of the new Postgres app to create.

The new cluster runs in the source primary's region with the same image
(unless --image-ref is set), machine size and volume size. Use
--restore-target-time or --restore-target-name to restore to a specific
time or backup. It works only on clusters managed by repmgr (Flex), the default for
` + "`fly postgres create`" + `, and only after ` + "`fly postgres backup enable`" + `.`

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

	flapsClient := flapsutil.ClientFromContext(ctx)

	machines, err := flapsClient.ListActive(ctx, appName)
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

	enabled, err := isBackupEnabled(ctx, flapsClient, appName)
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

	out, err := flapsClient.Exec(ctx, appName, leader.ID, in)
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
	app, err := flapsClient.GetApp(ctx, appName)
	if err != nil {
		return err
	}
	org, err := uiexutil.AppOrganization(ctx, app)
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
		Network:                   flapsutil.NetworkName(app),
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

	return launcher.LaunchMachinesPostgres(ctx, input, false)
}

func newBackupCreate() *cobra.Command {
	const (
		short = "Create a backup of a Fly Postgres cluster"
		long  = `Create a backup of a Fly Postgres cluster by running it on the primary.
The Postgres app comes from --app or fly.toml. The primary needs at least
512MB of memory. It works only on clusters managed by repmgr (Flex), the default for
` + "`fly postgres create`" + `, and only after ` + "`fly postgres backup enable`" + `.

See existing backups with ` + "`fly postgres backup list`" + `.`

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

	flapsClient := flapsutil.ClientFromContext(ctx)

	machines, err := flapsClient.ListActive(ctx, appName)
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

	enabled, err := isBackupEnabled(ctx, flapsClient, appName)
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

	return ExecOnLeader(ctx, flapsClient, appName, cmd)
}

func newBackupEnable() *cobra.Command {
	const (
		short = "Enable backups on a Fly Postgres cluster"
		long  = `Enable WAL-based backups on a Fly Postgres cluster. This creates a Tigris
bucket for the backups and stores its details as a secret on the
Postgres app, which comes from --app or fly.toml.

The cluster must be managed by repmgr (Flex), the default for
` + "`fly postgres create`" + `, and its primary needs at least 512MB of memory. The
new configuration takes effect after you restart the cluster with
` + "`fly secrets deploy`" + `.`

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
	)

	flapsClient := flapsutil.ClientFromContext(ctx)

	app, err := flapsClient.GetApp(ctx, appName)
	if err != nil {
		return err
	}

	if !flapsutil.IsPostgresApp(app) {
		return fmt.Errorf("app %s is not a postgres app", appName)
	}

	// Check to see if backups are already enabled
	enabled, err := isBackupEnabled(ctx, flapsClient, appName)
	if err != nil {
		return err
	}

	// Short-circuit if backups are already enabled.
	if enabled {
		return fmt.Errorf("backups are already enabled")
	}

	machines, err := flapsClient.ListActive(ctx, appName)
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

	// CreateTigrisBucket provisions against the app, so the organization is
	// resolved server-side from AppName and is not needed here.
	pgInput := &flypg.CreateClusterInput{
		AppName:        appName,
		BackupsEnabled: true,
	}

	if err := flypg.CreateTigrisBucket(ctx, pgInput); err != nil {
		return fmt.Errorf("failed to create tigris bucket: %w", err)
	}

	secrets := map[string]string{
		flypg.BarmanSecretName: pgInput.BarmanSecret,
	}

	if err := appsecrets.Update(ctx, flapsClient, appName, secrets, nil); err != nil {
		return fmt.Errorf("failed to set secrets: %w", err)
	}

	fmt.Fprintf(io.Out, "Backups enabled. Run `fly secrets deploy -a %s` to restart the cluster with the new configuration.\n", appName)

	return nil
}

func newBackupList() *cobra.Command {
	const (
		short = "List backups of a Fly Postgres cluster"
		long  = `List the backups of a Fly Postgres cluster. The Postgres app comes from
--app or fly.toml. It works only on clusters managed by repmgr (Flex), the default for
` + "`fly postgres create`" + `, and only after ` + "`fly postgres backup enable`" + `.

Restore one into a new cluster with ` + "`fly postgres backup restore`" + `.`

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

	flapsClient := flapsutil.ClientFromContext(ctx)

	machines, err := flapsClient.ListActive(ctx, appName)
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

	enabled, err := isBackupEnabled(ctx, flapsClient, appName)
	if err != nil {
		return err
	}

	if !enabled {
		return fmt.Errorf("backups are not enabled. Run `fly pg backup enable -a %s` to enable them", appName)
	}

	if err = hasRequiredFlexVersionOnMachines(appName, machines, backupVersion); err != nil {
		return err
	}

	return ExecOnMachine(ctx, flapsClient, appName, machine.ID, "flexctl backup list")
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

func isBackupEnabled(ctx context.Context, flapsClient flapsutil.FlapsClient, appName string) (bool, error) {
	secrets, err := appsecrets.List(ctx, flapsClient, appName)
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
		long  = `Show and change how a Fly Postgres cluster takes and keeps backups: the
archive timeout, recovery window, full backup frequency and minimum
redundancy. Backups must already be enabled with
` + "`fly postgres backup enable`" + `.`
	)

	cmd := command.New("config", short, long, nil)

	cmd.AddCommand(newBackupConfigShow(), newBackupConfigUpdate())

	return cmd
}

func newBackupConfigShow() *cobra.Command {
	const (
		short = "Show backup configuration"
		long  = `Show the backup configuration of a Fly Postgres cluster, read from its
primary. The Postgres app comes from --app or fly.toml. It works only on clusters managed by repmgr (Flex), the default for
` + "`fly postgres create`" + `, and only after ` + "`fly postgres backup enable`" + `.

Change the settings with ` + "`fly postgres backup config update`" + `.`
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
		long  = `Update the backup configuration of a Fly Postgres cluster. Only the
settings you pass as flags change; they're applied on the primary. The
Postgres app comes from --app or fly.toml. It works only on clusters managed by repmgr (Flex), the default for
` + "`fly postgres create`" + `, and only after ` + "`fly postgres backup enable`" + `.

See the current settings with ` + "`fly postgres backup config show`" + `.`

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

	flapsClient := flapsutil.ClientFromContext(ctx)

	machines, err := flapsClient.ListActive(ctx, appName)
	if err != nil {
		return fmt.Errorf("list of machines could not be retrieved: %w", err)
	}

	if len(machines) == 0 {
		return fmt.Errorf("No active machines")
	}

	if !IsFlex(machines[0]) {
		return fmt.Errorf("backups are only supported on Flexclusters")
	}

	enabled, err := isBackupEnabled(ctx, flapsClient, appName)
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

	return ExecOnLeader(ctx, flapsClient, appName, "flexctl backup config show")
}

func runBackupConfigUpdate(ctx context.Context) error {
	var (
		appName = appconfig.NameFromContext(ctx)
	)

	flapsClient := flapsutil.ClientFromContext(ctx)

	machines, err := flapsClient.ListActive(ctx, appName)
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

	enabled, err := isBackupEnabled(ctx, flapsClient, appName)
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

	return ExecOnLeader(ctx, flapsClient, appName, command)
}
