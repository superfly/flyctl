// Package profile implements the profile command chain, which manages named,
// isolated credential profiles so one machine can drive several Fly.io
// accounts without logging in and out.
package profile

import (
	"github.com/MakeNowJust/heredoc/v2"
	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/internal/command"
	profilelib "github.com/superfly/flyctl/internal/profile"
)

// New initializes and returns a new profile Command.
func New() *cobra.Command {
	const short = "Manage credential profiles for multiple Fly.io accounts"

	long := heredoc.Doc(`
		Profiles let a single machine hold credentials for several Fly.io
		accounts at once and pick between them per shell, per project or per
		command.

		Each profile is a complete, isolated flyctl config directory, so it
		carries its own access token, WireGuard peer state and agent. The
		"default" profile is the existing ~/.fly directory, which means an
		installation that has never used profiles keeps working unchanged.

		The profile in effect is resolved in this order:

		  1. FLY_CONFIG_DIR, which pins a config directory outright
		  2. the --profile flag
		  3. the FLY_PROFILE environment variable
		  4. the nearest .fly-profile file at or above the current directory
		  5. the profile selected by "fly profile use"
		  6. the "default" profile

		A profile named by any of those that does not exist is an error, never
		a silent fallback to another account.
	`)

	cmd := command.New("profile", short, long, nil)

	cmd.Aliases = []string{"profiles"}

	cmd.AddCommand(
		newList(),
		newAdd(),
		newUse(),
		newShow(),
		newLink(),
		newRemove(),
		newRename(),
	)

	// A .fly-profile file or an active pointer left naming a deleted profile
	// makes every other command refuse to run. These are the commands that
	// repair that, so they must survive it.
	for _, sub := range cmd.Commands() {
		if sub.Annotations == nil {
			sub.Annotations = map[string]string{}
		}

		sub.Annotations[profilelib.TolerateUnresolvedAnnotation] = "1"
	}

	return cmd
}
