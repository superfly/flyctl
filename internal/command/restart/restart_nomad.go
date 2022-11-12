package restart

import (
	"context"
	"fmt"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/internal/command/postgres"
	"github.com/superfly/flyctl/iostreams"
)

func runNomadRestart(ctx context.Context, app *api.AppCompact) error {
	if app.PostgresAppRole != nil && app.PostgresAppRole.Name == "postgres_cluster" {
		return postgres.RestartNomad(ctx, app)
	}

	return runNomadBaseRestart(ctx, app)
}

func runNomadBaseRestart(ctx context.Context, app *api.AppCompact) error {
	client := client.FromContext(ctx).API()

	if _, err := client.RestartApp(ctx, app.Name); err != nil {
		return fmt.Errorf("failed restarting app: %w", err)
	}

	io := iostreams.FromContext(ctx)
	fmt.Fprintf(io.Out, "%s is being restarted\n", app.Name)

	return nil
}
