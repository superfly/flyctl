package scale

import (
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
)

func newScaleShow() *cobra.Command {
	const (
		short = "Show current resources"
		long  = `Show current VM size and counts`
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
