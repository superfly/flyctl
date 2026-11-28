// Package cli implements the command line interface.
package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"html/template"
	io2 "io"
	"os"
	"regexp"
	"runtime/debug"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/AlecAivazis/survey/v2/terminal"
	"github.com/MakeNowJust/heredoc/v2"
	"github.com/kr/text"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	clientsignals "github.com/superfly/client-signals/go"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/command"
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
	tracecmd "github.com/superfly/flyctl/internal/command/trace"
)

// Run runs the command line interface with the given arguments and reports the
// exit code the application should exit with.
//
// Run changes process-wide output state (os.Stdout and the default logger). In
// text mode it restores that state when it returns. In JSON mode it leaves
// both pointing at stderr, because background work that outlives the 5s task
// shutdown (metrics, for one) may still log after Run returns and before main
// exits. A panic never restores it, so the crash report main prints follows
// the same routing.
func Run(ctx context.Context, io *iostreams.IOStreams, args ...string) int {
	stdout, defaultLogger := os.Stdout, term2.DefaultLogger
	code, jsonMode := run(ctx, io, args...)
	if !jsonMode {
		os.Stdout, term2.DefaultLogger = stdout, defaultLogger
	}

	return code
}

func run(ctx context.Context, io *iostreams.IOStreams, args ...string) (code int, jsonMode bool) {
	setNeverPromptFromEnv(io)
	ctx = iostreams.NewContext(ctx, io)

	// The client signals look at os.Stdout to tell whether a person is at a
	// terminal, so detect them while it is still the process's real stdout.
	clientsignals.DetectOnce()

	// Nothing may reach stdout before we know whether it is reserved for a JSON
	// document, which takes loading the config and building the command tree.
	held, decided := holdStdout(), false
	defer func() {
		// Only a panic gets here undecided. Treat it as JSON mode: diagnostics
		// on stderr, and the crash report main prints with them.
		if !decided {
			held.release(io.ErrOut)
			os.Stdout = os.Stderr
		}
	}()

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

	// Special case for the launch command, support `flyctl launch args -- [subargs]`
	// and `flyctl mcp wrap --mcp script -- [subargs]`
	// Where the arguments after `--` are passed to the scanner/dockerfile generator.
	// This isn't supported natively by cobra, so we have to manually split the args
	// See: https://github.com/spf13/cobra/issues/739
	if (len(args) > 0 && args[0] == "launch") || (len(args) > 2 && args[0] == "mcp" && args[1] == "wrap") {
		index := slices.Index(args, "--")
		if index >= 0 {
			ctx = flag.WithExtraArgs(ctx, args[index+1:])
			args = args[:index]
		}
	}

	// diagOut is where usage and other non-error text goes.
	diagOut := io.Out
	decided = true
	jsonMode = stdoutJSONMode(cmd, args)
	if jsonMode {
		// From here on, the only writer that reaches the real stdout is io.Out,
		// which the command uses for its JSON document. Everything else that
		// writes to os.Stdout or the default logger lands on stderr.
		held.release(io.ErrOut)
		os.Stdout = os.Stderr
		term2.SetLogOutput(io.ErrOut)
		diagOut = io.ErrOut
	} else {
		held.release(nil)
	}

	if v := os.Getenv(testPanicEnv); v != "" {
		panic(v)
	}

	cmd.SetOut(diagOut)
	cmd.SetErr(io.ErrOut)

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

	cmd, err = root.ExecuteContextC(ctx, cmd)

	if cmd != nil {
		metrics.RecordCommandFinish(cmd, err != nil)
	}

	// shutdown background tasks, giving up to 5s for them to finish
	task.FromContext(ctx).ShutdownWithTimeout(5 * time.Second)

	var childExit *tracecmd.ExitError
	switch {
	case errors.As(err, &childExit):
		return childExit.Code, jsonMode
	case err == nil:
		return 0, jsonMode
	case errors.Is(err, context.Canceled), errors.Is(err, terminal.InterruptErr):
		return 127, jsonMode
	case errors.Is(err, context.DeadlineExceeded):
		printError(io, cs, cmd, err)

		return 126, jsonMode
	case isUnchangedError(err):
		// This means the deployment was a noop, which is noteworthy but not something we should
		// fail CI on. Print a warning and exit 0. Remove this once we're fully on Machines!
		printError(io, cs, cmd, err)

		return 0, jsonMode
	default:
		printError(io, cs, cmd, err)

		_, _, e := cmd.Find(args)
		if e != nil {
			fmt.Fprintf(diagOut, "Run '%v --help' for usage.\n", cmd.CommandPath())
			fmt.Fprintln(diagOut)
		}

		return 1, jsonMode
	}
}

// testPanicEnv makes the CLI panic once its output routing is decided, so
// subprocess tests can check where the crash report goes.
const testPanicEnv = "FLYCTL_TEST_PANIC"

// heldStdout collects what the process writes to stdout, directly or through
// the default logger, until release.
type heldStdout struct {
	stdout *os.File
	logger *logger.Logger
	w      *os.File
	done   chan struct{}
	buf    bytes.Buffer
}

func holdStdout() *heldStdout {
	r, w, err := os.Pipe()
	if err != nil {
		return nil // nothing is held, so text goes where it always went
	}
	h := &heldStdout{stdout: os.Stdout, logger: term2.DefaultLogger, w: w, done: make(chan struct{})}
	go func() {
		defer close(h.done)
		_, _ = io2.Copy(&h.buf, r)
		_ = r.Close()
	}()
	os.Stdout = w
	term2.SetLogOutput(w)

	return h
}

// release restores stdout and the default logger, and writes what was held to
// dst, or to the original stdout when dst is nil.
func (h *heldStdout) release(dst io2.Writer) {
	if h == nil {
		return
	}
	os.Stdout, term2.DefaultLogger = h.stdout, h.logger
	_ = h.w.Close()
	<-h.done
	if dst == nil {
		dst = h.stdout
	}
	_, _ = dst.Write(h.buf.Bytes())
}

// stdoutJSONMode reports whether args run a command marked with
// command.StdoutJSONAnnotation in JSON mode: --json is true or malformed (so
// that the usage error stays off stdout), or it is unset and FLY_JSON is true.
// Cobra resolves the command and parses the flags, into copies so that the
// real parse is unaffected. When another flag fails to parse, the JSON flag's
// own occurrences in the arguments decide, so a usage error without JSON
// intent stays where it always was.
func stdoutJSONMode(root *cobra.Command, args []string) bool {
	target, rest, err := root.Find(args)
	if err != nil || target == nil || target.Annotations[command.StdoutJSONAnnotation] != "true" {
		return false
	}

	target.InitDefaultHelpFlag()
	target.InitDefaultVersionFlag()
	_ = target.InheritedFlags() // merges the parents' persistent flags into target.Flags()

	fs := pflag.NewFlagSet(target.Name(), pflag.ContinueOnError)
	fs.SetOutput(io2.Discard)
	fs.SetNormalizeFunc(target.Flags().GetNormalizeFunc())
	fs.ParseErrorsWhitelist = pflag.ParseErrorsWhitelist(target.FParseErrWhitelist)
	target.Flags().VisitAll(func(f *pflag.Flag) {
		fs.AddFlag(&pflag.Flag{
			Name:        f.Name,
			Shorthand:   f.Shorthand,
			NoOptDefVal: f.NoOptDefVal,
			Value:       &probeValue{typ: f.Value.Type(), val: f.DefValue},
		})
	})
	if target.DisableFlagParsing {
		return env.IsTruthy("FLY_JSON")
	}
	value, set := "", false
	if err := fs.Parse(rest); err != nil {
		value, set = jsonFlagValue(fs, rest)
	} else if fs.Changed(flagnames.JSONOutput) {
		value, set = fs.Lookup(flagnames.JSONOutput).Value.String(), true
	}
	if !set {
		return env.IsTruthy("FLY_JSON")
	}
	v, err := strconv.ParseBool(value)

	return err != nil || v
}

// jsonFlagValue finds the last value args give the JSON flag, walking them as
// pflag does: "--json" or "-j" grouped with other shorthands is "true", a
// "=value" is that value, and an argument consumed as another flag's value or
// following "--" is not a flag. Unknown flags are taken to have no value.
func jsonFlagValue(fs *pflag.FlagSet, args []string) (value string, set bool) {
	takesValue := func(f *pflag.Flag) bool { return f != nil && f.NoOptDefVal == "" }
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--":
			return value, set
		case strings.HasPrefix(a, "--"):
			name, v, hasValue := strings.Cut(a[2:], "=")
			f := fs.Lookup(name)
			switch {
			case f != nil && f.Name == flagnames.JSONOutput:
				value, set = "true", true
				if hasValue {
					value = v
				}
			case !hasValue && takesValue(f):
				i++ // the next argument is this flag's value
			}
		case strings.HasPrefix(a, "-") && len(a) > 1:
			shorthands := a[1:]
			for j := 0; j < len(shorthands); j++ {
				f := fs.ShorthandLookup(shorthands[j : j+1])
				if f == nil {
					break
				}
				rest := shorthands[j+1:]
				if f.Name == flagnames.JSONOutput {
					value, set = "true", true
					if strings.HasPrefix(rest, "=") {
						value = rest[1:]

						break
					}

					continue
				}
				if takesValue(f) {
					if rest == "" {
						i++ // the next argument is this flag's value
					}

					break
				}
				if strings.HasPrefix(rest, "=") {
					break
				}
			}
		}
	}

	return value, set
}

// probeValue stands in for a flag's value while stdoutJSONMode parses: it
// rejects what the real value would reject for the common scalar types, and
// keeps the raw text.
type probeValue struct{ typ, val string }

func (v *probeValue) String() string { return v.val }
func (v *probeValue) Type() string   { return v.typ }

func (v *probeValue) Set(s string) (err error) {
	switch v.typ {
	case "bool":
		_, err = strconv.ParseBool(s)
	case "int", "int8", "int16", "int32", "int64":
		_, err = strconv.ParseInt(s, 0, 64)
	case "uint", "uint8", "uint16", "uint32", "uint64":
		_, err = strconv.ParseUint(s, 0, 64)
	case "float32", "float64":
		_, err = strconv.ParseFloat(s, 64)
	case "duration":
		_, err = time.ParseDuration(s)
	}
	if err == nil {
		v.val = s
	}

	return err
}

// setNeverPromptFromEnv turns prompts off when FLY_NO_PROMPT is truthy, for
// callers such as coding agents that run flyctl under a pseudo-terminal that
// nobody answers.
func setNeverPromptFromEnv(io *iostreams.IOStreams) {
	if env.IsTruthy("FLY_NO_PROMPT") {
		io.SetNeverPrompt(true)
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
	if cmd != nil && env.IS_GH_ACTION() && env.IsTruthy("FLY_GHA_ERROR_ANNOTATION") {
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

	if cmd != nil {
		if bool, err := cmd.Flags().GetBool(flagnames.Debug); err == nil && bool {
			fmt.Fprintf(io.ErrOut, "Stacktrace:\n%s\n", debug.Stack())
		}
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
	return wrapParagraphs(heredoc.Doc(s), helpWidth()-1)
}

// wrapParagraphs reflows each blank-line-separated paragraph of s to width,
// keeping the blank lines between them. A paragraph with a line that is
// indented or starts like a list item is laid out line by line instead:
// list items longer than width wrap with a hanging indent under the item
// text, other indented lines (code, commands, tables) are never wrapped so
// they stay copyable, and other lines wrap normally.
func wrapParagraphs(s string, width int) string {
	var out []string
	for _, para := range paragraphBreak.Split(strings.TrimSpace(s), -1) {
		para = strings.Trim(para, "\n")
		if para == "" {
			continue
		}

		lines := strings.Split(para, "\n")
		if !isLineBased(lines) {
			out = append(out, text.Wrap(para, width))
			continue
		}

		for i, line := range lines {
			lines[i] = wrapLine(line, width)
		}
		out = append(out, strings.Join(lines, "\n"))
	}

	return strings.Join(out, "\n\n")
}

// wrapLine lays out one line of a line-based paragraph.
func wrapLine(line string, width int) string {
	if len(line) <= width {
		return line
	}

	if marker := listItem.FindString(line); marker != "" {
		hang := strings.Repeat(" ", len(marker))
		body := text.Wrap(line[len(marker):], width-len(marker))

		return marker + strings.ReplaceAll(body, "\n", "\n"+hang)
	}

	if isIndented(line) {
		return line
	}

	return text.Wrap(line, width)
}

var (
	paragraphBreak = regexp.MustCompile(`\n[ \t]*\n`)
	listItem       = regexp.MustCompile(`^[ \t]*(?:[-*•]|\d+[.)])[ \t]+`)
)

func isIndented(line string) bool {
	return strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t")
}

func isLineBased(lines []string) bool {
	for _, line := range lines {
		if isIndented(line) || listItem.MatchString(line) {
			return true
		}
	}

	return false
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
