package services

import (
	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/internal/command"
)

func New() *cobra.Command {
	const (
		long = `See the network services configured on an app's Machines. Run
` + "`fly services list`" + ` to list them.`
		short = `Show an app's services`
	)

	services := command.New("services", short, long, nil)

	services.AddCommand(
		newList(),
	)

	return services
}
