package root

import (
	"context"
	"errors"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flyerr"
)

// ExecuteContextC runs cmd and rejects unknown subcommands of help-only
// groups. Cobra falls back to help for such a group without validating its
// arguments, so the help function is wrapped for this call: it reads the
// arguments Cobra has already parsed and does not parse flags itself.
func ExecuteContextC(ctx context.Context, cmd *cobra.Command) (*cobra.Command, error) {
	help := cmd.HelpFunc()
	var unknown error
	cmd.SetHelpFunc(func(target *cobra.Command, argv []string) {
		positional := target.Flags().Args()
		explicitHelp, _ := target.Flags().GetBool("help")
		if len(positional) == 0 {
			// The CLI strips everything after "--" for launch before Cobra
			// runs, so `fly launch plan -- bogus` arrives here without it.
			positional = flag.ExtraArgsFromContext(ctx)
		}
		if !explicitHelp && !target.Runnable() && target.HasSubCommands() &&
			len(positional) > 0 && positional[0] != "help" {
			message := fmt.Sprintf("unknown command %q for %q", positional[0], target.CommandPath())
			if positional[0] != "" && !target.DisableSuggestions {
				if target.SuggestionsMinimumDistance <= 0 {
					target.SuggestionsMinimumDistance = 2
				}
				if suggestions := target.SuggestionsFor(positional[0]); len(suggestions) > 0 {
					message += "\n\nDid you mean this?\n"
					for _, suggestion := range suggestions {
						message += "\t" + suggestion + "\n"
					}
				}
			}
			unknown = flyerr.WithSuggestion(errors.New(message), fmt.Sprintf("Run '%s --help' for usage.", target.CommandPath()))

			return
		}
		help(target, argv)
	})
	defer cmd.SetHelpFunc(help)
	target, err := cmd.ExecuteContextC(ctx)
	if unknown != nil {
		return target, unknown
	}

	return target, err
}
