package postgres

import (
	"context"
	"fmt"
	"os"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/internal/cli/internal/command"
	"github.com/superfly/flyctl/internal/cli/internal/command/ssh"

	"github.com/superfly/flyctl/internal/cli/internal/flag"
	"github.com/superfly/flyctl/internal/client"
	"github.com/superfly/flyctl/pkg/agent"
)

func newConnect() (cmd *cobra.Command) {
	const (
		// TODO: document command
		long = `
			Connect to postgres instance.
		`
		short = "Establishs a session with Postgres"
		usage = "connect [APPNAME]"
	)

	cmd = command.New(usage, short, long, runConnect,
		command.RequireSession,
	)
	cmd.Args = cobra.MaximumNArgs(1)

	flag.Add(cmd,
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

	return
}

func runConnect(ctx context.Context) error {
	client := client.FromContext(ctx).API()

	appName := flag.FirstArg(ctx)

	app, err := client.GetApp(ctx, appName)
	if err != nil {
		return fmt.Errorf("get app: %w", err)
	}

	agentclient, err := agent.Establish(ctx, client)
	if err != nil {
		return errors.Wrap(err, "can't establish agent")
	}

	dialer, err := agentclient.Dialer(ctx, &app.Organization)
	if err != nil {
		return fmt.Errorf("failed to build tunnel for %s. %v", app.Organization.Slug, err)
	}

	database := flag.GetString(ctx, "database")
	user := flag.GetString(ctx, "user")
	password := flag.GetString(ctx, "password")

	addr := fmt.Sprintf("%s.internal", appName)
	cmdStr := fmt.Sprintf("connect %s %s %s", database, user, password)

	return ssh.SSHConnect(&ssh.SSHParams{
		Ctx:    ctx,
		Org:    &app.Organization,
		Dialer: dialer,
		App:    appName,
		Cmd:    cmdStr,
		Stdin:  os.Stdin,
		Stdout: os.Stdout,
		Stderr: os.Stderr,
	}, addr)

}
