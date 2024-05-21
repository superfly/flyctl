package apps

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/flag/completion"
	"github.com/superfly/flyctl/internal/flyutil"

	"github.com/superfly/flyctl/iostreams"

	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/prompt"
)

func newDestroy() *cobra.Command {
	const (
		long = "Delete an application from the Fly platform."

		short = "Permanently destroy an app."
		usage = "destroy <app name>"
	)

	destroy := command.New(usage, short, long, RunDestroy,
		command.RequireSession)

	destroy.Args = cobra.ExactArgs(1)

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
	appName := flag.FirstArg(ctx)
	client := flyutil.ClientFromContext(ctx)

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

	return nil
}
