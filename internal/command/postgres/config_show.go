package postgres

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/agent"
	"github.com/superfly/flyctl/flypg"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/command/apps"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flyutil"
	mach "github.com/superfly/flyctl/internal/machine"
	"github.com/superfly/flyctl/internal/render"
	"github.com/superfly/flyctl/iostreams"
)

func newConfigShow() (cmd *cobra.Command) {
	const (
		long  = `Show Postgres configuration`
		short = "Show Postgres configuration"
		usage = "show"
	)

	cmd = command.New(usage, short, long, runConfigShow,
		command.RequireSession,
		command.RequireAppName,
	)

	cmd.Aliases = []string{"view"}

	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
	)

	return
}

func runConfigShow(ctx context.Context) error {
	var (
		client  = flyutil.ClientFromContext(ctx)
		appName = appconfig.NameFromContext(ctx)
	)

	app, err := client.GetAppCompact(ctx, appName)
	if err != nil {
		return fmt.Errorf("failed retrieving app %s: %w", appName, err)
	}

	if !app.IsPostgresApp() {
		return fmt.Errorf("app %s is not a postgres app", appName)
	}

	ctx, err = apps.BuildContext(ctx, app)
	if err != nil {
		return err
	}
	return runMachineConfigShow(ctx, app)
}

func runMachineConfigShow(ctx context.Context, app *fly.AppCompact) (err error) {
	var (
		MinPostgresHaVersion         = "0.0.19"
		MinPostgresStandaloneVersion = "0.0.7"
		MinPostgresFlexVersion       = "0.0.3"
	)

	ctx, err = apps.BuildContext(ctx, app)
	if err != nil {
		return err
	}

	machines, err := mach.ListActive(ctx)
	if err != nil {
		return fmt.Errorf("machines could not be retrieved %w", err)
	}

	if err := hasRequiredVersionOnMachines(machines, MinPostgresHaVersion, MinPostgresFlexVersion, MinPostgresStandaloneVersion); err != nil {
		return err
	}

	leader, err := pickLeader(ctx, machines)
	if err != nil {
		return err
	}

	manager := flypg.StolonManager
	if IsFlex(leader) {
		manager = flypg.ReplicationManager
	}

	return showSettings(ctx, app, manager, leader.PrivateIP)
}

func showSettings(ctx context.Context, app *fly.AppCompact, manager string, leaderIP string) error {
	var (
		io       = iostreams.FromContext(ctx)
		colorize = io.ColorScheme()
		dialer   = agent.DialerFromContext(ctx)
	)

	pgclient := flypg.NewFromInstance(leaderIP, dialer)

	var settings []string
	for _, k := range pgSettings {
		settings = append(settings, k)
	}

	res, err := pgclient.ViewSettings(ctx, settings, manager)
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
			strings.ReplaceAll(setting.Name, "_", "-"),
			value,
			setting.Unit,
			desc,
			restart,
		})
	}
	_ = render.Table(io.Out, "", rows, "Name", "Value", "Unit", "Description", "Pending Restart")

	if pendingRestart {
		fmt.Fprintln(io.Out, colorize.Yellow("Some changes are awaiting a restart!"))
		fmt.Fprintln(io.Out, colorize.Yellow(fmt.Sprintf("To apply changes, run: `fly postgres restart --app %s`", app.Name)))
	}

	return nil
}
