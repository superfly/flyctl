package postgres

import (
	"context"
	"fmt"

	"github.com/r3labs/diff"
	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/internal/cli/internal/app"
	"github.com/superfly/flyctl/internal/cli/internal/command"
	"github.com/superfly/flyctl/internal/cli/internal/flag"
	"github.com/superfly/flyctl/internal/cli/internal/prompt"
	"github.com/superfly/flyctl/internal/cli/internal/render"
	"github.com/superfly/flyctl/internal/client"
	"github.com/superfly/flyctl/pkg/iostreams"
)

func newConfig() (cmd *cobra.Command) {
	// TODO - Add better top level docs.
	const (
		long = `
`
		short = ""
	)

	cmd = command.New("config", short, long, nil)

	cmd.AddCommand(
		newConfigView(),
		newConfigUpdate(),
	)

	return
}

// pgSettingMap maps the command-line arguments to the actual pgParameter.
var pgSettingMap = map[string]string{
	"wal-level":                  "wal_level",
	"max-connections":            "max_connections",
	"log-statement":              "log_statement",
	"log-min-duration-statement": "log_min_duration_statement",
}

func newConfigView() (cmd *cobra.Command) {
	const (
		long = `Configure postgres cluster
`
		short = "Configure postgres cluster"
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

func runConfigView(ctx context.Context) error {
	client := client.FromContext(ctx).API()
	appName := app.NameFromContext(ctx)

	io := iostreams.FromContext(ctx)
	colorize := io.ColorScheme()

	app, err := client.GetApp(ctx, appName)
	if err != nil {
		return fmt.Errorf("get app: %w", err)
	}

	pgCmd, err := newPostgresCmd(ctx, app)
	if err != nil {
		return err
	}

	var settings []string
	for _, k := range pgSettingMap {
		settings = append(settings, k)
	}

	resp, err := pgCmd.viewSettings(settings)
	if err != nil {
		return err
	}

	pendingRestart := false
	rows := make([][]string, 0, len(resp.Settings))
	for _, setting := range resp.Settings {
		restart := fmt.Sprint(setting.PendingRestart)
		if setting.PendingRestart {
			pendingRestart = true
			restart = colorize.Bold(restart)
		}
		rows = append(rows, []string{
			setting.Name,
			setting.Setting,
			setting.Desc,
			restart,
		})
	}
	_ = render.Table(io.Out, "", rows, "Name", "Value", "Desc", "Pending Restart")

	if pendingRestart {
		fmt.Fprintln(io.Out, colorize.Bold("Some changes are awaiting a restart!"))
		fmt.Fprintln(io.Out, colorize.Bold("Run `DEV=1 fly services postgres restart` to apply the changes."))
	}

	return nil
}

func newConfigUpdate() (cmd *cobra.Command) {
	const (
		long = `Manage Stolon and Postgres configuration.
`
		short = "Configure postgres cluster"
		usage = "update"
	)

	cmd = command.New(usage, short, long, runConfigUpdate,
		command.RequireSession,
		command.RequireAppName,
	)

	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
		flag.String{
			Name:        "max-connections",
			Description: "Sets the maximum number of concurrent connections.",
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
		flag.Bool{
			Name:        "auto-confirm",
			Description: "Will automatically confirm changes without an interactive prompt.",
		},
	)

	return
}

func runConfigUpdate(ctx context.Context) error {
	client := client.FromContext(ctx).API()
	appName := app.NameFromContext(ctx)

	app, err := client.GetApp(ctx, appName)
	if err != nil {
		return fmt.Errorf("get app: %w", err)
	}

	pgCmd, err := newPostgresCmd(ctx, app)
	if err != nil {
		return err
	}

	io := iostreams.FromContext(ctx)
	colorize := io.ColorScheme()

	// Identify requested configuration changes.
	rChanges := map[string]string{}
	keys := []string{}
	for key := range pgSettingMap {
		val := flag.GetString(ctx, key)
		if val != "" {
			rChanges[pgSettingMap[key]] = val
			keys = append(keys, pgSettingMap[key])
		}
	}

	// Pull existing configuration
	settings, err := pgCmd.viewSettings(keys)
	if err != nil {
		return err
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

	restartRequired := false

	rows := make([][]string, 0, len(changelog))
	for _, change := range changelog {
		requiresRestart := isRestartRequired(settings, change.Path[len(change.Path)-1])
		if requiresRestart {
			restartRequired = true
		}
		rows = append(rows, []string{
			change.Path[len(change.Path)-1],
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

	fmt.Fprintln(io.Out, "Performing update...")
	err = pgCmd.updateSettings(rChanges)
	if err != nil {
		return err
	}

	fmt.Fprintln(io.Out, "Update complete!")

	if restartRequired {
		fmt.Fprintln(io.Out, colorize.Bold("Please note that some of your changes will require a cluster restart before they will be applied."))
		fmt.Fprintln(io.Out, colorize.Bold("To review the state of your changes, please run: 'DEV=1 fly services postgres config view'"))
	}

	return nil
}

func isRestartRequired(pgSettings *pgSettings, setting string) bool {
	for _, s := range pgSettings.Settings {
		if s.Name == setting {
			if s.Context == "postmaster" {
				return true
			}
		}
	}

	return false
}
