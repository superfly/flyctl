package secrets

import (
	"context"
	"errors"
	"fmt"

	"github.com/spf13/cobra"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/helpers"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/appsecrets"
	"github.com/superfly/flyctl/internal/cmdutil"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flapsutil"
)

func newSet() (cmd *cobra.Command) {
	const (
		long  = `Set one or more encrypted secrets for an application`
		short = long
		usage = "set [flags] NAME=VALUE NAME=VALUE ..."
	)

	cmd = command.New(usage, short, long, runSet, command.RequireSession, command.RequireAppName)

	flag.Add(cmd,
		sharedFlags,
	)

	cmd.Args = cobra.MinimumNArgs(1)

	return cmd
}

func runSet(ctx context.Context) (err error) {
	appName := appconfig.NameFromContext(ctx)
	ctx, flapsClient, app, err := flapsutil.SetClient(ctx, appName)
	if err != nil {
		return err
	}

	secrets, err := cmdutil.ParseKVStringsToMap(flag.Args(ctx))
	if err != nil {
		return fmt.Errorf("could not parse secrets: %w", err)
	}

	for k, v := range secrets {
		if v == "-" {
			if !helpers.HasPipedStdin() {
				return fmt.Errorf("secret `%s` expects standard input but none provided", k)
			}
			inval, err := helpers.ReadStdin(64 * 1024)
			if err != nil {
				return fmt.Errorf("error reading stdin for '%s': %s", k, err)
			}
			secrets[k] = inval
		}
	}

	if len(secrets) < 1 {
		return errors.New("requires at least one SECRET=VALUE pair")
	}

	return SetSecretsAndDeploy(ctx, flapsClient, app, secrets, flag.GetBool(ctx, "stage"), flag.GetBool(ctx, "detach"))
}

// TODO: XXX: delete minvers when app is deleted
// TODO: XXX: use minvers for deploys

func SetSecretsAndDeploy(ctx context.Context, flapsClient flapsutil.FlapsClient, app *fly.AppCompact, secrets map[string]string, stage bool, detach bool) error {
	if err := appsecrets.Update(ctx, flapsClient, app.Name, secrets, nil); err != nil {
		return fmt.Errorf("update secrets: %w", err)
	}

	return DeploySecrets(ctx, app, stage, detach)
}
