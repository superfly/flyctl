package postgres

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/agent"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/internal/app"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/command/recipe"
	"github.com/superfly/flyctl/internal/command/ssh"
	"github.com/superfly/flyctl/internal/flag"
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
		MinPostgresStandaloneVersion = "0.0.4"
		MinPostgresHaVersion         = "0.0.9"
		appName                      = app.NameFromContext(ctx)
		client                       = client.FromContext(ctx).API()
	)

	app, err := client.GetAppCompact(ctx, appName)
	if err != nil {
		return fmt.Errorf("failed retrieving app %s: %w", appName, err)
	}

	database := flag.GetString(ctx, "database")
	user := flag.GetString(ctx, "user")
	password := flag.GetString(ctx, "password")

	cmdStr := fmt.Sprintf("connect %s %s %s", database, user, password)

	switch app.PlatformVersion {
	case "nomad":

		if !app.IsPostgresApp() {
			return fmt.Errorf("app %s is not a postgres app", appName)
		}

		agentclient, err := agent.Establish(ctx, client)
		if err != nil {
			return fmt.Errorf("failed to establish agent: %w", err)
		}

		dialer, err := agentclient.Dialer(ctx, app.Organization.Slug)
		if err != nil {
			return fmt.Errorf("failed to build tunnel for %s: %v", app.Organization.Slug, err)
		}
		ctx = agent.DialerWithContext(ctx, dialer)
		if err := hasRequiredVersionOnNomad(app, MinPostgresHaVersion, MinPostgresStandaloneVersion); err != nil {
			return err
		}

		pgInstances, err := agentclient.Instances(ctx, app.Organization.Slug, app.Name)
		if err != nil {
			return fmt.Errorf("failed to lookup 6pn ip for %s app: %v", app.Name, err)
		}

		if len(pgInstances.Addresses) == 0 {
			return fmt.Errorf("no 6pn ips found for %s app", app.Name)
		}

		leaderIP, err := leaderIpFromNomadInstances(ctx, pgInstances.Addresses)
		if err != nil {
			return err
		}

		return ssh.SSHConnect(&ssh.SSHParams{
			Ctx:    ctx,
			Org:    app.Organization,
			Dialer: dialer,
			App:    app.Name,
			Cmd:    cmdStr,
			Stdin:  os.Stdin,
			Stdout: os.Stdout,
			Stderr: os.Stderr,
		}, leaderIP)

	case "machines":

		template := recipe.RecipeTemplate{
			Name: "Postgres connect",
			App:  app,
			Constraints: recipe.Constraints{
				AppRoleID: "postgres_cluster",
				Images: []recipe.ImageRequirements{
					{
						Repository:    "flyio/postgres",
						MinFlyVersion: MinPostgresHaVersion,
					},
					{
						Repository:    "flyio/postgres-standalone",
						MinFlyVersion: MinPostgresStandaloneVersion,
					},
				},
			},
			Operations: []*recipe.Operation{
				{
					Name: "connect",
					Type: recipe.CommandTypeSSHConnect,
					SSHConnectCommand: recipe.SSHConnectCommand{
						Command: fmt.Sprintf("connect %s %s %s",
							database, user, password),
					},
					Selector: recipe.Selector{
						HealthCheck: recipe.HealthCheckSelector{
							Name:  "role",
							Value: "leader",
						},
					},
				},
			},
		}

		if err = template.Process(ctx); err != nil {
			return err
		}

		return nil

	default:
		return fmt.Errorf("platform %s is not supported", app.PlatformVersion)
	}

}
