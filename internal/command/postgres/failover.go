package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/internal/app"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/command/recipe"
	"github.com/superfly/flyctl/internal/flag"
)

func newFailover() *cobra.Command {
	const (
		short = "Failover to a new primary"
		long  = short + "\n"
		usage = "failover"
	)

	cmd := command.New(usage, short, long, runFailover,
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

func runFailover(ctx context.Context) error {
	var (
		MinPostgresHaVersion = "0.0.20"
		client               = client.FromContext(ctx).API()
		appName              = app.NameFromContext(ctx)
	)

	app, err := client.GetAppCompact(ctx, appName)
	if err != nil {
		return fmt.Errorf("get app: %w", err)
	}

	template := recipe.RecipeTemplate{
		Name:         "Rolling restart",
		App:          app,
		RequireLease: true,
		Constraints: recipe.Constraints{
			AppRoleID:       "postgres_cluster",
			PlatformVersion: "machines",
			Images: []recipe.ImageRequirements{
				{
					Repository:    "flyio/postgres",
					MinFlyVersion: MinPostgresHaVersion,
				},
			},
		},
		Operations: []*recipe.Operation{
			{
				Name: "failover",
				Type: recipe.CommandTypeHTTP,
				HTTPCommand: recipe.HTTPCommand{
					Method:   "GET",
					Endpoint: "/commands/admin/failover/trigger",
					Port:     5500,
				},
				Selector: recipe.Selector{
					HealthCheck: recipe.HealthCheckSelector{
						Name:  "role",
						Value: "leader",
					},
				},
			},
			{
				Name: "wait-for-leader-change",
				Type: recipe.CommandTypeWaitFor,
				WaitForCommand: recipe.WaitForCommand{
					HealthCheck: recipe.HealthCheckSelector{
						Name:  "role",
						Value: "replica",
					},
					Retries:  30,
					Interval: time.Second,
				},
				Selector: recipe.Selector{
					HealthCheck: recipe.HealthCheckSelector{
						Name:  "role",
						Value: "leader",
					},
					Preprocess: true,
				},
			},
		},
	}

	if err := template.Process(ctx); err != nil {
		return err
	}

	return nil
}
