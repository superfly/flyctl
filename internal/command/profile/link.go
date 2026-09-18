package profile

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	profilelib "github.com/superfly/flyctl/internal/profile"
	"github.com/superfly/flyctl/iostreams"
)

func newLink() *cobra.Command {
	const (
		short = "Pin the current directory tree to a profile"
		long  = `Writes a .fly-profile file in the current directory naming the
profile to use for it and everything beneath it.

This is what makes multi-account work painless: once a project is linked, any
flyctl command run inside it reaches the right account with no switching and
no flags, no matter which profile is active globally.

Commit the file to source control to share the binding with the rest of the
team, or add it to .gitignore to keep it personal.
`
	)

	cmd := command.New("link <name>", short, long, runLink)

	cmd.Aliases = []string{"bind"}
	cmd.Args = cobra.ExactArgs(1)

	return cmd
}

func runLink(ctx context.Context) error {
	io := iostreams.FromContext(ctx)
	cs := io.ColorScheme()

	name := flag.FirstArg(ctx)

	if err := profilelib.ValidateName(name); err != nil {
		return err
	}

	exists, err := profilelib.Exists(name)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("profile %q does not exist; create it with `fly profile add %s`", name, name)
	}

	wd, err := os.Getwd()
	if err != nil {
		return err
	}

	path, err := profilelib.WriteProjectFile(wd, name)
	if err != nil {
		return err
	}

	fmt.Fprintf(io.Out, "wrote %s\n", path)
	fmt.Fprintf(io.Out, "flyctl commands under %s will now use profile %s\n", wd, cs.Bold(name))

	return nil
}
