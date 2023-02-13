// Package cli implements the command line interface.
package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/AlecAivazis/survey/v2/terminal"
	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/iostreams"
	"github.com/superfly/graphql"

	"github.com/superfly/flyctl/internal/flyerr"
	"github.com/superfly/flyctl/internal/logger"

	"github.com/superfly/flyctl/internal/command/root"
)

// Run runs the command line interface with the given arguments and reports the
// exit code the application should exit with.
func Run(ctx context.Context, io *iostreams.IOStreams, args ...string) int {
	ctx = iostreams.NewContext(ctx, io)
	ctx = logger.NewContext(ctx, logger.FromEnv(io.ErrOut))

	cmd := root.New()
	cmd.SetOut(io.Out)
	cmd.SetErr(io.ErrOut)
	cmd.SetArgs(args)

	cs := io.ColorScheme()

	switch _, err := cmd.ExecuteContextC(ctx); {
	case err == nil:
		return 0
	case errors.Is(err, context.Canceled), errors.Is(err, terminal.InterruptErr):
		return 127
	case errors.Is(err, context.DeadlineExceeded):
		printError(io.ErrOut, cs, err)

		return 126
	case isUnchangedError(err):
		// This means the deployment was a noop, which is noteworthy but not something we should
		// fail CI on. Print a warning and exit 0. Remove this once we're fully on Machines!
		printError(io.ErrOut, cs, err)
		return 0
	default:
		printError(io.ErrOut, cs, err)

		return 1
	}
}

// isUnchangedError returns true if the error returned is an UNCHANGED GraphQL error.
// Remove this once we're fully on Machines!
func isUnchangedError(err error) bool {
	var gqlErr *graphql.GraphQLError

	if errors.As(err, &gqlErr) {
		return gqlErr.Extensions.Code == "UNCHANGED"
	}
	return false
}

func printError(w io.Writer, cs *iostreams.ColorScheme, err error) {
	var b bytes.Buffer

	fmt.Fprintln(&b, cs.Red("Error"), err)
	fmt.Fprintln(&b)

	description := flyerr.GetErrorDescription(err)
	if description != "" {
		fmt.Fprintf(&b, "\n%s", description)
	}

	suggestion := flyerr.GetErrorSuggestion(err)
	if suggestion != "" {
		if description != "" {
			fmt.Fprintln(&b)
		}

		fmt.Fprintf(&b, "\n%s", suggestion)
	}
	fmt.Fprintln(&b)

	_, _ = b.WriteTo(w)
}

// TODO: remove this once generation of the docs has been refactored.
func NewRootCommand() *cobra.Command {
	return root.New()
}
