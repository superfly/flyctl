package postgres

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"

	"github.com/pkg/errors"
	"github.com/r3labs/diff"
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/agent"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/flaps"
	"github.com/superfly/flyctl/flypg"
	"github.com/superfly/flyctl/internal/app"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/prompt"
	"github.com/superfly/flyctl/internal/render"
	"github.com/superfly/flyctl/iostreams"
)

// pgSettings maps the command-line argument to the actual pgParameter.
// This also acts as a whitelist as far as what's configurable via flyctl and
// can be expanded on as needed.
var pgSettings = map[string]string{
	"wal-level":                  "wal_level",
	"max-connections":            "max_connections",
	"shared-buffers":             "shared_buffers",
	"log-statement":              "log_statement",
	"log-min-duration-statement": "log_min_duration_statement",
	"shared-preload-libraries":   "shared_preload_libraries",
}

func newConfig() (cmd *cobra.Command) {
	// TODO - Add better top level docs.
	const (
		short = "View and manage Postgres configuration."
		long  = short + "\n"
	)

	cmd = command.New("config", short, long, nil)

	cmd.AddCommand(
		newConfigView(),
		newConfigUpdate(),
	)

	return
}

func newConfigView() (cmd *cobra.Command) {
	const (
		long = `View your Postgres configuration
`
		short = "View your Postgres configuration"
		usage = "view"
	)

	cmd = command.New(usage, short, long, runConfigView,
		command.RequireSession,
		command.RequireAppName,
	)

	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
	)

	return
}

func runConfigView(ctx context.Context) (err error) {
	var (
		client   = client.FromContext(ctx).API()
		appName  = app.NameFromContext(ctx)
		io       = iostreams.FromContext(ctx)
		colorize = io.ColorScheme()
	)

	MinPostgresHaVersion := "0.0.19"

	app, err := client.GetAppCompact(ctx, appName)
	if err != nil {
		return fmt.Errorf("get app: %w", err)
	}

	if !app.IsPostgresApp() {
		return fmt.Errorf("app %s is not a postgres app", app.Name)
	}

	agentclient, err := agent.Establish(ctx, client)
	if err != nil {
		return errors.Wrap(err, "can't establish agent")
	}

	dialer, err := agentclient.Dialer(ctx, app.Organization.Slug)
	if err != nil {
		return fmt.Errorf("ssh: can't build tunnel for %s: %s", app.Organization.Slug, err)
	}
	ctx = agent.DialerWithContext(ctx, dialer)

	var firstPgIp net.IP
	switch app.PlatformVersion {
	case "nomad":
		if err := hasRequiredVersionOnNomad(app, MinPostgresHaVersion, MinPostgresHaVersion); err != nil {
			return err
		}
		pgInstances, err := agentclient.Instances(ctx, app.Organization.Slug, app.Name)
		if err != nil {
			return fmt.Errorf("failed to lookup 6pn ip for %s app: %v", app.Name, err)
		}
		if len(pgInstances.Addresses) == 0 {
			return fmt.Errorf("no 6pn ips found for %s app", app.Name)
		}
		firstPgIp = net.ParseIP(pgInstances.Addresses[0])
	case "machines":
		flapsClient, err := flaps.New(ctx, app)
		if err != nil {
			return fmt.Errorf("list of machines could not be retrieved: %w", err)
		}

		members, err := flapsClient.ListActive(ctx)
		if err != nil {
			return fmt.Errorf("machines could not be retrieved %w", err)
		}
		if err := hasRequiredVersionOnMachines(members, MinPostgresHaVersion, MinPostgresHaVersion); err != nil {
			return err
		}
		leader, _ := machinesNodeRoles(ctx, members)
		firstPgIp = net.ParseIP(leader.PrivateIP)
	}

	pgclient := flypg.NewFromInstance(firstPgIp.String(), dialer)

	var settings []string
	for _, k := range pgSettings {
		settings = append(settings, k)
	}

	res, err := pgclient.SettingsView(ctx, settings)
	if err != nil {
		return err
	}

	pendingRestart := false

	rows := make([][]string, 0, len(res.Settings))
	for _, setting := range res.Settings {
		desc := setting.Desc
		switch setting.VarType {
		case "enum":
			e := strings.Join(setting.EnumVals, ", ")
			desc = fmt.Sprintf("%s [%s]", desc, e)
		case "integer":
			desc = fmt.Sprintf("%s (%s, %s)", desc, setting.MinVal, setting.MaxVal)
		case "real":
			min, err := strconv.ParseFloat(setting.MinVal, 32)
			if err != nil {
				return nil
			}
			max, err := strconv.ParseFloat(setting.MaxVal, 32)
			if err != nil {
				return err
			}
			desc = fmt.Sprintf("%s (%.1f, %.1f)", desc, min, max)
		case "bool":
			desc = fmt.Sprintf("%s [on, off]", desc)

		}

		value := setting.Setting
		restart := fmt.Sprint(setting.PendingRestart)
		if setting.PendingRestart {
			pendingRestart = true
			restart = colorize.Bold(restart)
		}
		if setting.PendingChange != "" {
			p := colorize.Bold(fmt.Sprintf("(%s)", setting.PendingChange))
			value = fmt.Sprintf("%s -> %s", value, p)
		}
		rows = append(rows, []string{
			strings.Replace(setting.Name, "_", "-", -1),
			value,
			setting.Unit,
			desc,
			restart,
		})
	}
	_ = render.Table(io.Out, "", rows, "Name", "Value", "Unit", "Description", "Pending Restart")

	if pendingRestart {
		fmt.Fprintln(io.Out, colorize.Yellow("Some changes are awaiting a restart!"))
		fmt.Fprintln(io.Out, colorize.Yellow(fmt.Sprintf("To apply changes, run: `fly postgres restart --app %s`", appName)))
	}

	return
}

func newConfigUpdate() (cmd *cobra.Command) {
	const (
		long = `Update Postgres configuration.
`
		short = "Update Postgres configuration."
		usage = "update"
	)

	cmd = command.New(usage, short, long, runConfigUpdate,
		command.RequireSession,
		command.RequireAppName,
	)

	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
		flag.Detach(),
		flag.String{
			Name:        "max-connections",
			Description: "Sets the maximum number of concurrent connections.",
		},
		flag.String{
			Name:        "shared-buffers",
			Description: "Sets the amount of memory the database server uses for shared memory buffers",
		},
		flag.String{
			Name:        "wal-level",
			Description: "Sets the level of information written to the WAL. (minimal, replica, logical).",
		},
		flag.String{
			Name:        "log-statement",
			Description: "Sets the type of statements logged. (none, ddl, mod, all)",
		},
		flag.String{
			Name:        "log-min-duration-statement",
			Description: "Sets the minimum execution time above which all statements will be logged. (ms)",
		},
		flag.String{
			Name:        "shared-preload-libraries",
			Description: "Sets the shared libraries to preload. (comma separated string)",
		},
		flag.Bool{
			Name:        "auto-confirm",
			Description: "Will automatically confirm changes without an interactive prompt.",
		},
		flag.Bool{
			Name:        "force",
			Description: "Skips pg-setting value verification.",
		},
		flag.Bool{
			Name:        "confirm-restart",
			Description: "Will automatically confirm restart without an interactive prompt.",
		},
	)

	return
}

func runConfigUpdate(ctx context.Context) (err error) {
	var (
		client   = client.FromContext(ctx).API()
		appName  = app.NameFromContext(ctx)
		io       = iostreams.FromContext(ctx)
		colorize = io.ColorScheme()
	)

	app, err := client.GetAppCompact(ctx, appName)
	if err != nil {
		return fmt.Errorf("get app: %w", err)
	}

	cmd, err := flypg.NewCommand(ctx, app)
	if err != nil {
		return err
	}

	ctx = flypg.CommandWithContext(ctx, cmd)

	// Identify requested configuration changes.
	rChanges := map[string]string{}
	keys := []string{}
	for key := range pgSettings {
		val := flag.GetString(ctx, key)
		if val != "" {
			rChanges[pgSettings[key]] = val
			keys = append(keys, pgSettings[key])
		}
	}

	if len(rChanges) == 0 {
		return fmt.Errorf("no changes were specified")
	}

	agentclient, err := agent.Establish(ctx, client)
	if err != nil {
		return errors.Wrap(err, "can't establish agent")
	}

	dialer, err := agentclient.Dialer(ctx, app.Organization.Slug)
	if err != nil {
		return fmt.Errorf("ssh: can't build tunnel for %s: %s", app.Organization.Slug, err)
	}

	ctx = agent.DialerWithContext(ctx, dialer)

	flapsClient, err := flaps.New(ctx, app)
	if err != nil {
		return err
	}
	ctx = flaps.NewContext(ctx, flapsClient)

	pgInstances, err := agentclient.Instances(ctx, app.Organization.Slug, app.Name)
	if err != nil {
		return fmt.Errorf("failed to lookup 6pn ip for %s app: %v", app.Name, err)
	}
	if len(pgInstances.Addresses) == 0 {
		return fmt.Errorf("no 6pn ips found for %s app", app.Name)
	}
	leaderIp, err := leaderIpFromNomadInstances(ctx, pgInstances.Addresses)
	if err != nil {
		return err
	}
	pgclient := flypg.NewFromInstance(leaderIp, dialer)

	force := flag.GetBool(ctx, "force")

	restartRequired := false

	if !force {
		settings, err := pgclient.SettingsView(ctx, keys)
		if err != nil {
			return err
		}
		// Verfiy that input values are within acceptible ranges.
		// Stolon does not verify this, so we need to do it here.
		for k, v := range rChanges {
			for _, setting := range settings.Settings {
				if setting.Name == k {
					if err = validateConfigValue(setting, k, v); err != nil {
						return err
					}
				}
			}
		}

		// Construct a map of the active configuration settings so we can compare.
		oValues := map[string]string{}
		for _, setting := range settings.Settings {
			oValues[setting.Name] = setting.Setting
		}

		// Calculate diff
		changelog, _ := diff.Diff(oValues, rChanges)
		if len(changelog) == 0 {
			return fmt.Errorf("no changes to apply")
		}

		rows := make([][]string, 0, len(changelog))
		for _, change := range changelog {
			requiresRestart := isRestartRequired(settings, change.Path[len(change.Path)-1])
			if requiresRestart {
				restartRequired = true
			}
			name := strings.Replace(change.Path[len(change.Path)-1], "_", "-", -1)

			rows = append(rows, []string{
				name,
				fmt.Sprint(change.From),
				fmt.Sprint(change.To),
				fmt.Sprint(requiresRestart),
			})
		}
		_ = render.Table(io.Out, "", rows, "Name", "Value", "Target value", "Restart Required")

		if !flag.GetBool(ctx, "auto-confirm") {
			const msg = "Are you sure you want to apply these changes?"

			switch confirmed, err := prompt.Confirmf(ctx, msg); {
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
	}

	switch app.PlatformVersion {
	case "nomad":
		if err = updateNomadConfig(ctx, app, rChanges); err != nil {
			return err
		}
	case "machines":
		if err := updateMachinesConfig(ctx, app, rChanges); err != nil {
			return fmt.Errorf("error updating config: %w", err)
		}
	case "":
		return fmt.Errorf("app %s has an invalid platform flag", app.Name)
	}

	fmt.Fprintln(io.Out, "Update complete!")

	if restartRequired {
		fmt.Fprintln(io.Out, colorize.Yellow("Please note that some of your changes will require a cluster restart before they will be applied."))
		fmt.Fprintln(io.Out, colorize.Yellow("To review the state of your changes, run: `fly postgres config view`"))

		if !flag.GetBool(ctx, "confirm-restart") {
			switch confirmed, err := prompt.Confirm(ctx, "Restart cluster now?"); {
			case err == nil:
				if !confirmed {
					return nil
				}
			case prompt.IsNonInteractive(err):
				return prompt.NonInteractiveError("confirm-restart flag must be specified when not running interactively")
			default:
				return err
			}
		}

		switch app.PlatformVersion {
		case "nomad":
			if err := NomadRestart(ctx, app); err != nil {
				return err
			}
		case "machines":
			if err := MachinesRestart(ctx); err != nil {
				return fmt.Errorf("error restarting cluster: %w", err)
			}
		default:
			return fmt.Errorf("app %s has an invalid platform flag", app.Name)
		}

	}

	return
}

func updateMachinesConfig(ctx context.Context, app *api.AppCompact, changes map[string]string) (err error) {
	var (
		io     = iostreams.FromContext(ctx)
		dialer = agent.DialerFromContext(ctx)
		cmd    = flypg.CommandFromContext(ctx)
		fclt   = flaps.FromContext(ctx)
	)

	machines, err := fclt.ListActive(ctx)
	if err != nil {
		return fmt.Errorf("machines could not be retrieved")
	}

	var leader *api.Machine

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

		if role == "leader" {
			leader = machine
			break
		} else if role == "replica" {
			continue
		}
	}

	if leader == nil {
		return fmt.Errorf("no leader found")
	}

	// obtain lease on leader
	flaps, err := flaps.New(ctx, app)
	if err != nil {
		return err
	}

	// get lease on machine
	lease, err := flaps.GetLease(ctx, leader.ID, api.IntPointer(40))
	if err != nil {
		return fmt.Errorf("failed to obtain lease: %w", err)
	}
	defer flaps.ReleaseLease(ctx, leader.ID, lease.Data.Nonce)

	fmt.Fprintf(io.Out, "Acquired lease %s on machine: %s\n", lease.Data.Nonce, leader.ID)
	fmt.Fprintln(io.Out, "Performing update...")

	err = cmd.UpdateSettings(ctx, leader.PrivateIP, changes)
	if err != nil {
		return err
	}

	return
}

func updateNomadConfig(ctx context.Context, app *api.AppCompact, changes map[string]string) (err error) {
	var (
		io     = iostreams.FromContext(ctx)
		cmd    = flypg.CommandFromContext(ctx)
		client = client.FromContext(ctx).API()
	)

	agentclient, err := agent.Establish(ctx, client)
	if err != nil {
		return errors.Wrap(err, "can't establish agent")
	}

	pgInstances, err := agentclient.Instances(ctx, app.Organization.Slug, app.Name)
	if err != nil {
		return fmt.Errorf("failed to lookup 6pn ip for %s app: %v", app.Name, err)
	}
	if len(pgInstances.Addresses) == 0 {
		return fmt.Errorf("no 6pn ips found for %s app", app.Name)
	}
	leaderIp, err := leaderIpFromNomadInstances(ctx, pgInstances.Addresses)
	if err != nil {
		return err
	}

	fmt.Fprintln(io.Out, "Performing update...")

	err = cmd.UpdateSettings(ctx, leaderIp, changes)
	if err != nil {
		return err
	}
	return
}

func isRestartRequired(pgSettings *flypg.PGSettings, name string) bool {
	for _, s := range pgSettings.Settings {
		if s.Name == name {
			if s.Context == "postmaster" {
				return true
			}
		}
	}

	return false
}

func validateConfigValue(setting flypg.PGSetting, key, val string) error {
	switch setting.VarType {
	case "enum":
		for _, enumVal := range setting.EnumVals {
			if enumVal == val {
				return nil
			}
		}
		return fmt.Errorf("invalid value specified for %s. Received: %s, Accepted values: [%s]", key, val, strings.Join(setting.EnumVals, ", "))
	case "integer":
		min, err := strconv.Atoi(setting.MinVal)
		if err != nil {
			return err
		}
		max, err := strconv.Atoi(setting.MaxVal)
		if err != nil {
			return err
		}

		v, err := strconv.Atoi(val)
		if err != nil {
			return err
		}

		if v < min || v > max {
			return fmt.Errorf("invalid value specified for %s. (Received: %s, Accepted range: (%s, %s)", key, val, setting.MinVal, setting.MaxVal)
		}
	case "real":
		min, err := strconv.ParseFloat(setting.MinVal, 32)
		if err != nil {
			return err
		}

		max, err := strconv.ParseFloat(setting.MaxVal, 32)
		if err != nil {
			return err
		}

		v, err := strconv.ParseFloat(val, 32)
		if err != nil {
			return err
		}

		if v < min || v > max {
			return fmt.Errorf("invalid value specified for %s. (Received: %s, Accepted range: (%.1f, %.1f)", key, val, min, max)
		}
	case "bool":
		if val != "on" && val != "off" {
			return fmt.Errorf("invalid value specified for %s. (Received: %s, Accepted values: [on, off]", key, val)
		}
	}

	return nil
}
