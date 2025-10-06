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
		long  = `Set one or more encrypted secrets for an application. Values are read from stdin as NAME=VALUE pairs`
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
	ctx, flapsClient, app, err := flapsutil.SetClient(ctx, nil, appName)
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
