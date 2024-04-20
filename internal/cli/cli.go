// Package cli implements the command line interface.
package cli

import (
	"context"
	"errors"
	"fmt"
	"html/template"
	"os"
	"runtime/debug"
	"slices"
	"strings"
	"time"

	"github.com/AlecAivazis/survey/v2/terminal"
	"github.com/MakeNowJust/heredoc/v2"
	"github.com/kr/text"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/env"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flag/flagnames"
	"github.com/superfly/flyctl/internal/flyerr"
	"github.com/superfly/flyctl/internal/metrics"
	"github.com/superfly/flyctl/internal/task"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/term"

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

	// Special case for the launch command, support `flyctl launch args -- [subargs]`
	// Where the arguments after `--` are passed to the scanner/dockerfile generator.
	// This isn't supported natively by cobra, so we have to manually split the args
	// See: https://github.com/spf13/cobra/issues/739
	if len(args) > 0 && args[0] == "launch" {
		index := slices.Index(args, "--")
		if index >= 0 {
			ctx = flag.WithExtraArgs(ctx, args[index+1:])
			args = args[:index]
		}
	}

	cmd.SetArgs(args)
	cmd.SilenceErrors = true

	// configure help templates and helpers
	cobra.AddTemplateFuncs(template.FuncMap{
		"wrapFlagUsages": wrapFlagUsages,
		"wrapText":       wrapText,
	})
	cmd.SetUsageTemplate(usageTemplate)
	cmd.SetHelpTemplate(helpTemplate)

	cs := io.ColorScheme()

	cmd, err = cmd.ExecuteContextC(ctx)

	if cmd != nil {
		metrics.RecordCommandFinish(cmd, err != nil)
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

func isValidTraceID(id string) bool {
	t, err := trace.TraceIDFromHex(id)
	if err != nil {
		return false
	}
	return t.IsValid()
}

func printError(io *iostreams.IOStreams, cs *iostreams.ColorScheme, cmd *cobra.Command, err error) {
	if env.IS_GH_ACTION() && env.IsTruthy("FLY_GHA_ERROR_ANNOTATION") {
		printGHAErrorAnnotation(cmd, err)
	}

	var requestId, traceID string

	if requestId = flaps.GetErrorRequestID(err); requestId != "" {
		requestId = fmt.Sprintf(" (Request ID: %s)", requestId)
	}

	traceID = flaps.GetErrorTraceID(err)
	if isValidTraceID(traceID) {
		traceID = fmt.Sprintf(" (Trace ID: %s)", traceID)
	} else {
		traceID = ""
	}

	fmt.Fprint(io.ErrOut, cs.Red("Error: "), err.Error(), requestId, traceID, "\n")

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

func printGHAErrorAnnotation(cmd *cobra.Command, err error) {
	errMsg := err.Error()
	if requestId := flaps.GetErrorRequestID(err); requestId != "" {
		errMsg += " (Request ID: " + requestId + ")"
	}

	if description := flyerr.GetErrorDescription(err); description != "" && err.Error() != description {
		errMsg += "\n" + description
	}

	// GHA annotation messages don't support multiple lines. replace \n with a symbol to prevent losing output
	//
	errMsg = strings.ReplaceAll(errMsg, "\n", "⏎")

	fmt.Printf("::error title=flyctl error::%s\n", errMsg)
}

// TODO: remove this once generation of the docs has been refactored.
func NewRootCommand() *cobra.Command {
	return root.New()
}

func wrapFlagUsages(cmd *pflag.FlagSet) string {
	width := helpWidth()

	return cmd.FlagUsagesWrapped(width - 1)
}

func wrapText(s string) string {
	width := helpWidth()

	return strings.TrimSpace(text.Wrap(heredoc.Doc(s), width-1))
}

func helpWidth() int {
	fd := int(os.Stdout.Fd())
	width := 80

	// Get the terminal width and dynamically set
	termWidth, _, err := term.GetSize(fd)
	if err == nil {
		width = termWidth
	}

	return min(120, width)
}

// identical to the default cobra help template, but utilizes wrapText
// https://github.com/spf13/cobra/blob/fd865a44e3c48afeb6a6dbddadb8a5519173e029/command.go#L580-L582
const helpTemplate = `{{with (or .Long .Short)}}{{. | trimTrailingWhitespaces | wrapText}}

{{end}}{{if or .Runnable .HasSubCommands}}{{.UsageString}}{{end}}`

// identical to the default cobra usage template, but utilizes wrapFlagUsages
// https://github.com/spf13/cobra/blob/fd865a44e3c48afeb6a6dbddadb8a5519173e029/command.go#L539-L568
const usageTemplate = `Usage:{{if .Runnable}}
  {{.UseLine}}{{end}}{{if .HasAvailableSubCommands}}
  {{.CommandPath}} [command]{{end}}{{if gt (len .Aliases) 0}}

Aliases:
  {{.NameAndAliases}}{{end}}{{if .HasExample}}

Examples:
{{.Example}}{{end}}{{if .HasAvailableSubCommands}}{{$cmds := .Commands}}{{if eq (len .Groups) 0}}

Available Commands:{{range $cmds}}{{if (or .IsAvailableCommand (eq .Name "help"))}}
  {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}{{else}}{{range $group := .Groups}}

{{.Title}}{{range $cmds}}{{if (and (eq .GroupID $group.ID) (or .IsAvailableCommand (eq .Name "help")))}}
  {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}{{end}}{{if not .AllChildCommandsHaveGroup}}

Additional Commands:{{range $cmds}}{{if (and (eq .GroupID "") (or .IsAvailableCommand (eq .Name "help")))}}
  {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}{{end}}{{end}}{{end}}{{if .HasAvailableLocalFlags}}

Flags:
{{wrapFlagUsages .LocalFlags | trimTrailingWhitespaces}}{{end}}{{if .HasAvailableInheritedFlags}}

Global Flags:
{{wrapFlagUsages .InheritedFlags | trimTrailingWhitespaces}}{{end}}{{if .HasHelpSubCommands}}

Additional help topics:{{range .Commands}}{{if .IsAdditionalHelpTopicCommand}}
  {{rpad .CommandPath .CommandPathPadding}} {{.Short}}{{end}}{{end}}{{end}}{{if .HasAvailableSubCommands}}

Use "{{.CommandPath}} [command] --help" for more information about a command.{{end}}
`
