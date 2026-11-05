package profile

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	profilelib "github.com/superfly/flyctl/internal/profile"
	"github.com/superfly/flyctl/iostreams"
)

func newRename() *cobra.Command {
	const (
		short = "Rename a profile"
		long  = `Renames a stored profile, carrying the active selection across
if it pointed at the renamed profile.

Any .fly-profile file naming the old profile still names the old profile, so
re-link those projects afterwards.
`
	)

	cmd := command.New("rename <old-name> <new-name>", short, long, runRename)

	cmd.Args = cobra.ExactArgs(2)

	return cmd
}

func runRename(ctx context.Context) error {
	io := iostreams.FromContext(ctx)
	cs := io.ColorScheme()

	args := flag.Args(ctx)
	oldName, newName := args[0], args[1]

	if err := profilelib.ValidateName(newName); err != nil {
		return err
	}

	if err := profilelib.Rename(oldName, newName); err != nil {
		return err
	}

	fmt.Fprintf(io.Out, "renamed profile %s to %s\n", cs.Bold(oldName), cs.Bold(newName))

	return nil
}
