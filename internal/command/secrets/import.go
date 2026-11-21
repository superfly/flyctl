package secrets

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flapsutil"
)

func newImport() (cmd *cobra.Command) {
	const (
		long = `Set one or more encrypted secrets for an app from standard input, one
NAME=VALUE pair per line, for example ` + "`fly secrets import < .env`" + `.
Blank lines and lines starting with # are skipped. Values can be wrapped in
single or double quotes, and triple double quotes (""") allow multi-line
values.

Like ` + "`fly secrets set`" + `, it then updates all of the app's existing Machines
with the currently deployed configuration and image, which restarts them.
With ` + "`--stage`" + `, the secrets are saved but no Machines are updated; run
` + "`fly secrets deploy`" + ` or ` + "`fly deploy`" + ` to apply them.`
		short = `Set secrets as NAME=VALUE pairs from stdin`
		usage = "import [flags]"
	)

	cmd = command.New(usage, short, long, runImport, command.RequireSession, command.RequireAppName)

	flag.Add(cmd,
		sharedFlags,
	)

	return cmd
}

func runImport(ctx context.Context) (err error) {
	appName := appconfig.NameFromContext(ctx)

	flapsClient := flapsutil.ClientFromContext(ctx)

	app, err := flapsClient.GetApp(ctx, appName)
	if err != nil {
		return err
	}

	secrets, err := parseSecrets(os.Stdin)
	if err != nil {
		return fmt.Errorf("Failed to parse secrets from stdin: %w", err)
	}
	if len(secrets) < 1 {
		return errors.New("requires at least one SECRET=VALUE pair")
	}

	return SetSecretsAndDeploy(ctx, flapsClient, app, secrets, DeploymentArgs{
		Stage:    flag.GetBool(ctx, "stage"),
		Detach:   flag.GetBool(ctx, "detach"),
		CheckDNS: flag.GetBool(ctx, "dns-checks"),
	})
}
