package secrets

import (
	"context"
	"fmt"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/command/deploy"
	"github.com/superfly/flyctl/internal/watch"
	"github.com/superfly/flyctl/iostreams"

	"github.com/spf13/cobra"
)

func New() *cobra.Command {

	const (
		long = `Secrets are provided to applications at runtime as ENV variables. Names are
		case sensitive and stored as-is, so ensure names are appropriate for
		the application and vm environment.
		`

		short = "Manage application secrets with the set and unset commands."
	)

	secrets := command.New("secrets", short, long, nil)

	secrets.AddCommand(
		newList(),
		newSet(),
		newUnset(),
		newImport(),
	)

	return secrets
}

func deployForSecrets(ctx context.Context, app *api.AppCompact, release *api.Release) (err error) {
	out := iostreams.FromContext(ctx).Out

	if app.PlatformVersion == "machines" {
		return deploy.DeployMachinesApp(ctx, app, "rolling", nil)
	}

	if !app.Deployed {
		fmt.Fprint(out, "Secrets are staged for the first deployment")
		return nil
	}

	fmt.Fprintf(out, "Release v%d created\n", release.Version)

	err = watch.Deployment(ctx, release.EvaluationID)

	return err
}
