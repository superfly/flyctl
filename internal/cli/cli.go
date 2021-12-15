// Package cli implements the command line interface.
package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/AlecAivazis/survey/v2/terminal"
	"github.com/logrusorgru/aurora"
	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/pkg/iostreams"

	"github.com/superfly/flyctl/internal/flyerr"
	"github.com/superfly/flyctl/internal/logger"

	"github.com/superfly/flyctl/internal/cli/internal/command/root"
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

	switch _, err := cmd.ExecuteContextC(ctx); {
	case err == nil:
		return 0
	case errors.Is(err, context.Canceled), errors.Is(err, terminal.InterruptErr):
		return 127
	case errors.Is(err, context.DeadlineExceeded):
		printError(io.ErrOut, err)

		return 126
	default:
		printError(io.ErrOut, err)

		return 1
	}
}

func printError(w io.Writer, err error) {
	var b bytes.Buffer

	fmt.Fprintln(&b, aurora.Red("Error"), err)
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
