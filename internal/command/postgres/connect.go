package postgres

import (
	"context"
	"fmt"
	"os"

	"github.com/docker/docker/pkg/ioutils"
	"github.com/mattn/go-colorable"
	"github.com/spf13/cobra"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/agent"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/command/apps"
	"github.com/superfly/flyctl/internal/command/ssh"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/flyutil"
)

func newConnect() *cobra.Command {
	const (
		short = "Connect to the Postgres console"
		long  = short + "\n"

		usage = "connect"
	)

	cmd := command.New(usage, short, long, runConnect,
		command.RequireSession,
		command.RequireAppName,
	)

	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
		flag.String{
			Name:        "database",
			Shorthand:   "d",
			Description: "The name of the database you would like to connect to",
			Default:     "postgres",
		},
		flag.String{
			Name:        "user",
			Shorthand:   "u",
			Description: "The postgres user to connect with",
			Default:     "postgres",
		},
		flag.String{
			Name:        "password",
			Shorthand:   "p",
			Description: "The postgres user password",
		},
	)

	return cmd
}

func runConnect(ctx context.Context) error {
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

	return runMachineConnect(ctx, app)
}

func runMachineConnect(ctx context.Context, app *fly.AppCompact) error {
	var (
		MinPostgresHaVersion         = "0.0.9"
		MinPostgresFlexVersion       = "0.0.3"
		MinPostgresStandaloneVersion = "0.0.4"

		database = flag.GetString(ctx, "database")
		user     = flag.GetString(ctx, "user")
		password = flag.GetString(ctx, "password")
	)

	flapsClient := flapsutil.ClientFromContext(ctx)

	machines, err := flapsClient.ListActive(ctx)
	if err != nil {
		return fmt.Errorf("machines could not be retrieved %w", err)
	}

	if err := hasRequiredVersionOnMachines(app.Name, machines, MinPostgresHaVersion, MinPostgresFlexVersion, MinPostgresStandaloneVersion); err != nil {
		return err
	}

	leader, err := pickLeader(ctx, machines)
	if err != nil {
		return err
	}
	return ssh.SSHConnect(&ssh.SSHParams{
		Ctx:      ctx,
		Org:      app.Organization,
		Dialer:   agent.DialerFromContext(ctx),
		App:      app.Name,
		Username: ssh.DefaultSshUsername,
		Cmd:      fmt.Sprintf("connect %s %s %s", database, user, password),
		Stdin:    os.Stdin,
		Stdout:   ioutils.NewWriteCloserWrapper(colorable.NewColorableStdout(), func() error { return nil }),
		Stderr:   ioutils.NewWriteCloserWrapper(colorable.NewColorableStderr(), func() error { return nil }),
	}, leader.PrivateIP)
}
