package restart

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/pkg/iostreams"

	"github.com/superfly/flyctl/internal/cli/internal/command"
	"github.com/superfly/flyctl/internal/cli/internal/flag"
	"github.com/superfly/flyctl/internal/client"
)

// TODO: deprecate
func New() *cobra.Command {
	const (
		long = `The RESTART command will restart all running vms. 
`
		short = "Restart an application"
		usage = "restart [APPNAME]"
	)

	restart := command.New(usage, short, long, run,
		command.RequireSession)

	restart.Args = cobra.ExactArgs(1)

	return restart
}

func run(ctx context.Context) error {
	client := client.FromContext(ctx).API()

	appName := flag.FirstArg(ctx)
	if _, err := client.RestartApp(ctx, appName); err != nil {
		return fmt.Errorf("failed restarting app: %w", err)
	}

	io := iostreams.FromContext(ctx)
	fmt.Fprintf(io.Out, "%s is being restarted\n", appName)

	return nil
}
