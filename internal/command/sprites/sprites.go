// Package sprites implements the `fly sprites` commands, which manage Sprites:
// persistent, hardware-isolated Linux environments. The commands mirror the
// standalone sprite CLI and share its .sprite file, so a directory set up with
// either tool works with both.
package sprites

import (
	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/internal/command"
)

func New() *cobra.Command {
	const (
		short = "Manage Sprites: persistent Linux environments for development and agents."

		long = `Manage Sprites: persistent, hardware-isolated Linux environments.

A sprite has its own filesystem, URL, services and network policy. It pauses
when idle and resumes on demand, and its filesystem can be checkpointed and
restored.

Commands act on the sprite named with --sprite, or the one set for the current
directory with 'fly sprites use'. They use your Fly.io login; set SPRITE_TOKEN
to use a Sprites API token instead.
`
	)

	cmd := command.New("sprites", short, long, nil)
	cmd.Aliases = []string{"sprite"}

	cmd.AddCommand(
		newCreate(),
		newList(),
		newInfo(),
		newUse(),
		newDestroy(),
		newConfig(),
		newURL(),
		newCheckpoint(),
		newRestore(),
		newExec(),
		newConsole(),
		newAttach(),
		newSessions(),
		newFile(),
		newProxy(),
		newAPI(),
	)

	return cmd
}
