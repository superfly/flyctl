package wafris

import (
	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/internal/command"
)

func New() (cmd *cobra.Command) {

	const (
		short = "Provision and manage Wafris WAFs (Web Application Firewalls)"
		long  = `Provision and manage Wafris web application firewalls for your Fly apps.
Each app can have one. Create it with ` + "`fly ext wafris create`" + ` and open
its dashboard with ` + "`fly ext wafris dashboard`" + `.`
	)

	cmd = command.New("wafris", short, long, nil)
	cmd.Aliases = []string{"waf"}
	cmd.AddCommand(create(), destroy(), dashboard())

	return cmd
}
