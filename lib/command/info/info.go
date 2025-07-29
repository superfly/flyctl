package info

import (
	"github.com/superfly/flyctl/lib/command"
	"github.com/superfly/flyctl/lib/flag"

	"github.com/spf13/cobra"
)

func New() *cobra.Command {
	const (
		long  = `Shows information about the application.`
		short = `Shows information about the application`
	)

	cmd := command.New("info", short, long, nil)
	cmd.Hidden = true
	cmd.Deprecated = "Replaced by 'status', 'ips list', and 'services list'"

	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
	)

	return cmd
}
