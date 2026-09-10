package profile

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	profilelib "github.com/superfly/flyctl/internal/profile"
	"github.com/superfly/flyctl/internal/prompt"
	"github.com/superfly/flyctl/iostreams"
)

func newRemove() *cobra.Command {
	const (
		short = "Delete a profile and its stored credentials"
		long  = `Deletes the profile's config directory, including its access
token, WireGuard peer state and cached account details.

This only removes local credentials. It does not touch the Fly.io account, its
organizations or its apps. If the profile was active, the active profile falls
back to "default".
`
	)

	cmd := command.New("remove <name>", short, long, runRemove)

	cmd.Aliases = []string{"rm", "delete"}
	cmd.Args = cobra.ExactArgs(1)

	flag.Add(cmd, flag.Yes())

	return cmd
}

func runRemove(ctx context.Context) error {
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
		return fmt.Errorf("profile %q does not exist", name)
	}

	dir, err := profilelib.Dir(name)
	if err != nil {
		return err
	}

	if !flag.GetYes(ctx) {
		md, _ := profilelib.ReadMetadata(name)

		msg := fmt.Sprintf("Delete profile %q and its credentials in %s?", name, dir)
		if md.Email != "" {
			msg = fmt.Sprintf("Delete profile %q (%s) and its credentials in %s?", name, md.Email, dir)
		}

		switch confirmed, err := prompt.Confirm(ctx, msg); {
		case err != nil:
			return err
		case !confirmed:
			return nil
		}
	}

	if err := profilelib.Remove(name); err != nil {
		return err
	}

	fmt.Fprintf(io.Out, "removed profile %s\n", cs.Bold(name))

	return nil
}
