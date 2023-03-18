package secrets

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
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
	client := client.FromContext(ctx).API()
	appName := appconfig.NameFromContext(ctx)
	app, err := client.GetAppCompact(ctx, appName)
	if err != nil {
		return
	}

	secrets, err := parseSecrets(os.Stdin)
	if err != nil {
		return fmt.Errorf("Failed to parse secrets from stdin: %w", err)
	}
	if len(secrets) < 1 {
		return errors.New("requires at least one SECRET=VALUE pair")
	}

	release, err := client.SetSecrets(ctx, appName, secrets)
	if err != nil {
		return err
	}

	return deployForSecrets(ctx, app, release)
}
