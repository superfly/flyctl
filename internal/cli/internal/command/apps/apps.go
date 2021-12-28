// Package apps implements the apps command chain.
package apps

import (
	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/internal/cli/internal/command"
)

// New initializes and returns a new apps Command.
func New() *cobra.Command {
	const (
		long = `The APPS commands focus on managing your Fly applications.
Start with the CREATE command to register your application.
The LIST command will list all currently registered applications.
`
		short = "Manage apps"
	)

	// TODO: list should also accept the --org param

	apps := command.New("apps", short, long, nil)

	apps.AddCommand(
		newList(),
		newCreate(),
		newDestroy(),
		newMove(),
		newSuspend(),
		newResume(),
		newRestart(),
		NewOpen(),
		NewReleases(),
	)

	return apps
}
