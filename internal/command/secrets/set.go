package secrets

import (
	"context"
	"errors"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/helpers"
	"github.com/superfly/flyctl/internal/app"
	"github.com/superfly/flyctl/internal/cmdutil"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/watch"
	"github.com/superfly/flyctl/iostreams"
)

func newSet() (cmd *cobra.Command) {
	const (
		long  = `Set one or more encrypted secrets for an application`
		short = long
		usage = "set [flags] NAME=VALUE NAME=VALUE ..."
	)

	cmd = command.New(usage, short, long, runSet, command.RequireSession, command.LoadAppNameIfPresent)

	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
	)

	cmd.Args = cobra.MinimumNArgs(1)

	return cmd
}

func runSet(ctx context.Context) (err error) {
	client := client.FromContext(ctx).API()
	appName := app.NameFromContext(ctx)
	out := iostreams.FromContext(ctx).Out
	app, err := client.GetAppCompact(ctx, appName)

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

	release, err := client.SetSecrets(ctx, appName, secrets)

	if err != nil {
		return err
	}

	if !app.Deployed {
		fmt.Fprint(out, "Secrets are staged for the first deployment")
		return nil
	}

	fmt.Fprintf(out, "Release v%d created\n", release.Version)

	err = watch.Deployment(ctx, release.EvaluationID)

	return err
}
