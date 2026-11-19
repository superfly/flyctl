package scale

import (
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
)

func newScaleShow() *cobra.Command {
	const (
		short = "Show current resources"
		long  = `Show the VM size (CPU kind, CPUs and memory) and Machine count of each of
the app's process groups, with the count per region. The size shown for
a group comes from one of its Machines. Change these with
` + "`fly scale count`" + ` and ` + "`fly scale vm`" + `.`
	)
	cmd := command.New("show", short, long, runMachinesScaleShow,
		command.RequireSession,
		command.RequireAppName,
	)
	cmd.Args = cobra.NoArgs
	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
		flag.JSONOutput(),
	)

	return cmd
}
