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

func newUse() *cobra.Command {
	const (
		short = "Switch the active profile"
		long  = `Selects the profile every subsequent flyctl command uses, unless
something more specific overrides it: the --profile flag, the FLY_PROFILE
environment variable and a .fly-profile file all win over this setting.

Switching to "default" restores the original ~/.fly credentials.
`
	)

	cmd := command.New("use <name>", short, long, runUse)

	cmd.Aliases = []string{"switch"}
	cmd.Args = cobra.ExactArgs(1)

	return cmd
}

func runUse(ctx context.Context) error {
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

	if err := profilelib.SetActive(name); err != nil {
		return err
	}

	md, _ := profilelib.ReadMetadata(name)
	if md.Email != "" {
		fmt.Fprintf(io.Out, "switched to profile %s (%s)\n", cs.Bold(name), md.Email)
	} else {
		fmt.Fprintf(io.Out, "switched to profile %s\n", cs.Bold(name))
	}

	// A narrower rule silently outranking the switch the user just made is
	// exactly the surprise this tool exists to prevent, so call it out.
	res, ok := profilelib.FromContext(ctx)
	switch {
	case !ok:
	case res.Err != nil:
		// Switching the active profile does not clear a dangling .fly-profile
		// or FLY_PROFILE, so the next command here would still fail.
		fmt.Fprintf(io.ErrOut, "%s %s\n", cs.Yellow("warning:"), res.Err)
	case res.Name != name:
		switch res.Source {
		case profilelib.SourceDefault, profilelib.SourceActive:
			// The old active pointer; the switch has already replaced it.
		default:
			selectedBy := string(res.Source)
			if res.Detail != "" {
				selectedBy = fmt.Sprintf("%s (%s)", res.Source, res.Detail)
			}

			fmt.Fprintf(io.ErrOut,
				"%s here, %s still takes precedence and selects profile %q\n",
				cs.Yellow("warning:"), selectedBy, res.Name,
			)
		}
	}

	return nil
}
