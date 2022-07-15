package secrets

import (
	"github.com/superfly/flyctl/internal/command"

	"github.com/spf13/cobra"
)

func New() *cobra.Command {

	const (
		long = `Secrets are provided to applications at runtime as ENV variables. Names are
		case sensitive and stored as-is, so ensure names are appropriate for
		the application and vm environment.
		`

		short = "Manage application secrets with the set and unset commands."
	)

	secrets := command.New("secrets", short, long, nil)

	secrets.AddCommand(
		newSet(),
	)

	return secrets
}
