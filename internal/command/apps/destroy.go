package apps

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/gql"
	"github.com/superfly/flyctl/internal/command/deploy/statics"
	"github.com/superfly/flyctl/internal/flag/completion"
	"github.com/superfly/flyctl/internal/flyutil"

	"github.com/superfly/flyctl/iostreams"

	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/prompt"
)

func newDestroy() *cobra.Command {
	const (
		long = "Delete one or more applications from the Fly platform."

		short = "Permanently destroy one or more apps."
		usage = "destroy <app name(s)>"
	)

	destroy := command.New(usage, short, long, RunDestroy,
		command.RequireSession)

	destroy.Args = cobra.ArbitraryArgs

	flag.Add(destroy,
		flag.Yes(),
	)

	destroy.ValidArgsFunction = completion.Adapt(completion.CompleteApps)

	destroy.Aliases = []string{"delete", "remove", "rm"}
	return destroy
}

// TODO: make internal once the destroy package is removed
func RunDestroy(ctx context.Context) error {
	io := iostreams.FromContext(ctx)
	colorize := io.ColorScheme()
	apps := flag.Args(ctx)
	client := flyutil.ClientFromContext(ctx)

	if len(apps) == 0 {
		return fmt.Errorf("no app names provided")
	}

	for _, appName := range apps {

		if !flag.GetYes(ctx) {
			const msg = "Destroying an app is not reversible."
			fmt.Fprintln(io.ErrOut, colorize.Red(msg))

			switch confirmed, err := prompt.Confirmf(ctx, "Destroy app %s?", appName); {
			case err == nil:
				if !confirmed {
					return nil
				}
			case prompt.IsNonInteractive(err):
				return prompt.NonInteractiveError("yes flag must be specified when not running interactively")
			default:
				return err
			}
		}

		app, err := client.GetApp(ctx, appName)
		if err != nil {
			return err
		}
		org, err := client.GetOrganizationBySlug(ctx, app.Organization.Slug)
		if err != nil {
			return err
		}

		bucket, err := statics.FindBucket(ctx, app, org)
		if err != nil {
			return err
		}

		if bucket != nil {
			_, err = gql.DeleteAddOn(ctx, client.GenqClient(), bucket.Name)
			if err != nil {
				return err
			}
			fmt.Fprintf(io.Out, "Destroyed statics bucket %s\n", bucket.Name)
		}

		if err := client.DeleteApp(ctx, appName); err != nil {
			return err
		}

		fmt.Fprintf(io.Out, "Destroyed app %s\n", appName)
	}

	return nil
}
