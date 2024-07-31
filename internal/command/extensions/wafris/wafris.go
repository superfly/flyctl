package wafris

import (
	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/internal/command"
)

func New() (cmd *cobra.Command) {

	const (
		short = "Provision and manage Wafris WAFs (Web Application Firewalls)"
		long  = short + "\n"
	)

	cmd = command.New("wafris", short, long, nil)
	cmd.Aliases = []string{"waf"}
	cmd.AddCommand(create(), destroy(), dashboard())

	return cmd
}
