package destroy

import (
	"context"
	"fmt"

	"github.com/logrusorgru/aurora"
	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/pkg/iostreams"

	"github.com/superfly/flyctl/internal/cli/internal/command"
	"github.com/superfly/flyctl/internal/cli/internal/flag"
	"github.com/superfly/flyctl/internal/cli/internal/prompt"
	"github.com/superfly/flyctl/internal/client"
)

// TODO: deprecate
func New() *cobra.Command {
	const (
		long = `The DESTROY command will remove an application 
from the Fly platform.
`
		short = "Permanently destroys an app"
		usage = "destroy [APPNAME]"
	)

	destroy := command.New(usage, short, long, run,
		command.RequireSession)

	destroy.Args = cobra.ExactArgs(1)

	flag.Add(destroy,
		flag.Yes(),
	)

	return destroy
}

func run(ctx context.Context) error {
	io := iostreams.FromContext(ctx)
	appName := flag.FirstArg(ctx)

	if !flag.GetYes(ctx) {
		fmt.Fprintln(io.ErrOut, aurora.Red("Destroying an app is not reversible."))

		msg := fmt.Sprintf("Destroy app %s?", appName)
		if confirmed, err := prompt.Confirm(ctx, msg); err != nil || !confirmed {
			return err
		}
	}

	client := client.FromContext(ctx).API()
	if err := client.DeleteApp(ctx, appName); err != nil {
		return err
	}

	fmt.Fprintf(io.Out, "Destroyed app %s\n", appName)

	return nil
}
