// Package cli implements the command line interface.
package cli

import (
	"context"
	"errors"
	"fmt"
	"runtime/debug"
	"time"

	"github.com/AlecAivazis/survey/v2/terminal"
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/flag/flagnames"
	"github.com/superfly/flyctl/internal/flyerr"
	"github.com/superfly/flyctl/internal/metrics"
	"github.com/superfly/flyctl/internal/task"

	"github.com/superfly/flyctl/iostreams"
	"github.com/superfly/graphql"

	"github.com/superfly/flyctl/internal/httptracing"
	"github.com/superfly/flyctl/internal/logger"

	term2 "github.com/superfly/flyctl/terminal"

	"github.com/superfly/flyctl/internal/command/root"
)

// Run runs the command line interface with the given arguments and reports the
// exit code the application should exit with.
func Run(ctx context.Context, io *iostreams.IOStreams, args ...string) int {
	ctx = iostreams.NewContext(ctx, io)

	err := logger.InitLogFile()
	if err != nil {
		term2.Debugf("failed to initialize file logger: %s", err)
	} else {
		defer func() {
			err := logger.CloseLogFile()
			if err != nil {
				term2.Debugf("failed to close file logger: %s", err)
			}
		}()
	}

	ctx = logger.NewContext(ctx, logger.FromEnv(io.ErrOut).AndLogToFile())
	// initialize the background task runner early so command preparers can start running stuff immediately
	ctx = task.NewWithContext(ctx)

	httptracing.Init()
	defer httptracing.Finish()

	cmd := root.New()
	cmd.SetOut(io.Out)
	cmd.SetErr(io.ErrOut)
	cmd.SetArgs(args)
	cmd.SilenceErrors = true

	cs := io.ColorScheme()

	cmd, err = cmd.ExecuteContextC(ctx)

	if err != nil {
		metrics.RecordCommandFinish(cmd)
	}

	// shutdown background tasks, giving up to 5s for them to finish
	task.FromContext(ctx).ShutdownWithTimeout(5 * time.Second)

	switch {
	case err == nil:
		return 0
	case errors.Is(err, context.Canceled), errors.Is(err, terminal.InterruptErr):
		return 127
	case errors.Is(err, context.DeadlineExceeded):
		printError(io, cs, cmd, err)
		return 126
	case isUnchangedError(err):
		// This means the deployment was a noop, which is noteworthy but not something we should
		// fail CI on. Print a warning and exit 0. Remove this once we're fully on Machines!
		printError(io, cs, cmd, err)
		return 0
	default:
		printError(io, cs, cmd, err)

		_, _, e := cmd.Find(args)
		if e != nil {
			fmt.Printf("Run '%v --help' for usage.\n", cmd.CommandPath())
			fmt.Println()
		}

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

func printError(io *iostreams.IOStreams, cs *iostreams.ColorScheme, cmd *cobra.Command, err error) {
	fmt.Fprint(io.ErrOut, cs.Red("Error: "))
	fmt.Fprint(io.Out, err.Error())

	fmt.Fprintln(io.ErrOut)

	if description := flyerr.GetErrorDescription(err); description != "" && err.Error() != description {
		fmt.Fprintln(io.ErrOut, description)
		fmt.Fprintln(io.ErrOut)
	}

	if suggestion := flyerr.GetErrorSuggestion(err); suggestion != "" {
		fmt.Fprintln(io.ErrOut, suggestion)
		fmt.Fprintln(io.ErrOut)
	}

	if docURL := flyerr.GetErrorDocUrl(err); docURL != "" {
		fmt.Fprintln(io.ErrOut, "View more information at ", docURL)
		fmt.Fprintln(io.ErrOut)
	}

	if bool, err := cmd.Flags().GetBool(flagnames.Debug); err == nil && bool {
		fmt.Fprintf(io.ErrOut, "Stacktrace:\n%s\n", debug.Stack())
	}

}

// TODO: remove this once generation of the docs has been refactored.
func NewRootCommand() *cobra.Command {
	return root.New()
}
