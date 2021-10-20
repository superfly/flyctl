// Package cli implements the command line interface.
package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/logrusorgru/aurora"

	"github.com/superfly/flyctl/pkg/iostreams"

	"github.com/superfly/flyctl/internal/cli/internal/root"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flyerr"
	"github.com/superfly/flyctl/internal/logger"
)

// Run runs the CLI with the given arguments and reports the exit code with
// which is application should exit.
func Run(ctx context.Context, io *iostreams.IOStreams, args ...string) int {
	ctx = iostreams.NewContext(ctx, io)

	l := logger.FromEnv(io.ErrOut)
	ctx = logger.NewContext(ctx, l)

	v, err := config.Load(l)
	if err != nil {
		return 3
	}

	cmd := root.New(v)
	cmd.SetOut(io.Out)
	cmd.SetErr(io.ErrOut)
	cmd.SetArgs(args)

	switch _, err := cmd.ExecuteContextC(ctx); {
	case err == nil:
		return 0
	case errors.Is(err, context.Canceled):
		return 127 // context was cancelled
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
