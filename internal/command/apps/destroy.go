package apps

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/iostreams"

	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/prompt"
)

func newDestroy() *cobra.Command {
	const (
		long = `The APPS DESTROY command will remove an application
from the Fly platform.
`
		short = "Permanently destroys an app"
		usage = "destroy <APPNAME>"
	)

	destroy := command.New(usage, short, long, RunDestroy,
		command.RequireSession)

	destroy.Args = cobra.ExactArgs(1)

	flag.Add(destroy,
		flag.Yes(),
	)

	return destroy
}

// TODO: make internal once the destroy package is removed
func RunDestroy(ctx context.Context) error {
	io := iostreams.FromContext(ctx)
	colorize := io.ColorScheme()
	appName := flag.FirstArg(ctx)
	client := client.FromContext(ctx).API()

	db_app_name := fmt.Sprintf("%s-db", appName)
	app_has_db := false
	var db_app api.App

	if apps, err := client.GetApps(ctx, nil); err == nil {
		for _, app := range apps {
			if app.Name == db_app_name {
				app_has_db = true
				db_app = app

			}

		}
	} else {
		return err
	}

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

	if err := client.DeleteApp(ctx, appName); err != nil {
		return err
	}

	fmt.Fprintf(io.Out, "Destroyed app %s\n", appName)

	destroy_db := false

	if app_has_db {
		if !flag.GetYes(ctx) {
			const msg = "This app also has a database. Should it be destroyed? This is not reversible."
			fmt.Fprintln(io.ErrOut, colorize.Red(msg))

			switch confirmed, err := prompt.Confirmf(ctx, "Destroy app %s?", db_app.Name); {
			case err == nil:
				destroy_db = confirmed
			case prompt.IsNonInteractive(err):
				return prompt.NonInteractiveError("yes flag must be specified when not running interactively")
			default:
				return err
			}

		}

	}

	if destroy_db && app_has_db {
		if err := client.DeleteApp(ctx, db_app.Name); err != nil {
			return err
		}

		fmt.Fprintf(io.Out, "Destroyed app %s\n", db_app.Name)
	}

	return nil
}
