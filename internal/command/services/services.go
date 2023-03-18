package services

import (
	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/internal/command"
)

func New() *cobra.Command {
	const (
		long  = `Shows information about the services of the application.`
		short = `Show the application's services`
	)

	services := command.New("services", short, long, nil)

	services.AddCommand(
		newList(),
	)

	return services
}
