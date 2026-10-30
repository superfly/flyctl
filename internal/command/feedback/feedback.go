// Package feedback implements the feedback command, which files a report with
// the Fly.io feedback API.
package feedback

import (
	"context"
	"fmt"
	"io"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/prompt"
	"github.com/superfly/flyctl/internal/render"
	"github.com/superfly/flyctl/internal/uiex"
	"github.com/superfly/flyctl/internal/uiexutil"
	"github.com/superfly/flyctl/iostreams"
)

const (
	maxNoteLength      = 4000
	maxRequestIDLength = 255
	maxURLLength       = 2048
)

var categories = []string{"bug", "documentation", "feature_request", "other"}

// New initializes and returns a new feedback Command.
func New() *cobra.Command {
	const (
		short = "Send feedback to Fly.io"
		long  = `Send a bug report, documentation issue, or feature request to Fly.io.

The note can be given as an argument, piped through stdin, or typed at the
prompt when running in a terminal. Tell us what you are reporting with
--request-id (the fly-request-id header of the API response in question) or
--url (the page or endpoint). At least one of them is required.

Reports are filed under the organization of the access token in use. A token
scoped to one organization files under that organization; any other token
files under your personal organization.`
		usage = "feedback [note]"
	)

	cmd := command.New(usage, short, long, run, command.RequireSession)
	cmd.Args = cobra.MaximumNArgs(1)

	flag.Add(cmd,
		flag.String{
			Name:        "category",
			Description: "Type of feedback. One of: " + strings.Join(categories, ", ") + ".",
			CompletionFn: func(_ context.Context, _ *cobra.Command, _ []string, _ string) ([]string, error) {
				return categories, nil
			},
		},
		flag.String{
			Name:        "request-id",
			Description: fmt.Sprintf("The fly-request-id of the request you are reporting (up to %d characters).", maxRequestIDLength),
		},
		flag.String{
			Name:        "url",
			Description: fmt.Sprintf("The page or endpoint you are reporting (up to %d characters).", maxURLLength),
		},
		flag.JSONOutput(),
	)

	return cmd
}

func run(ctx context.Context) error {
	in := uiex.FeedbackRequest{
		Category:  flag.GetString(ctx, "category"),
		RequestID: flag.GetString(ctx, "request-id"),
		URL:       flag.GetString(ctx, "url"),
	}

	// Check the flags before asking for a note, so a typo doesn't cost the
	// user a typed-out report.
	if err := validateTarget(in); err != nil {
		return err
	}

	note, err := readNote(ctx, flag.Args(ctx))
	if err != nil {
		return err
	}
	if err := validateNote(note); err != nil {
		return err
	}
	in.Note = note

	ios := iostreams.FromContext(ctx)

	out, err := uiexutil.ClientFromContext(ctx).SubmitFeedback(ctx, in)
	if err != nil {
		return err
	}

	if config.FromContext(ctx).JSONOutput {
		return render.JSON(ios.Out, out)
	}

	if out.AlreadySubmitted {
		fmt.Fprintf(ios.Out, "This feedback was already submitted (ID: %s)\n", out.FeedbackID)
	} else {
		fmt.Fprintf(ios.Out, "Thanks! Feedback submitted (ID: %s)\n", out.FeedbackID)
	}

	return nil
}

func validateTarget(in uiex.FeedbackRequest) error {
	switch {
	case in.Category == "":
		return fmt.Errorf("--category is required; one of: %s", strings.Join(categories, ", "))
	case !slices.Contains(categories, in.Category):
		return fmt.Errorf("invalid --category %q; must be one of: %s", in.Category, strings.Join(categories, ", "))
	case in.RequestID == "" && in.URL == "":
		return fmt.Errorf("at least one of --request-id or --url is required")
	case utf8.RuneCountInString(in.RequestID) > maxRequestIDLength:
		return fmt.Errorf("--request-id is too long (maximum %d characters)", maxRequestIDLength)
	case utf8.RuneCountInString(in.URL) > maxURLLength:
		return fmt.Errorf("--url is too long (maximum %d characters)", maxURLLength)
	}

	return nil
}

func validateNote(note string) error {
	if note == "" {
		return fmt.Errorf("the feedback note can't be empty")
	}
	if n := utf8.RuneCountInString(note); n > maxNoteLength {
		return fmt.Errorf("the feedback note is too long (%d characters, maximum %d)", n, maxNoteLength)
	}

	return nil
}

// readNote takes the note from the argument if given, else from piped stdin,
// else from a prompt when running in a terminal.
func readNote(ctx context.Context, args []string) (string, error) {
	if len(args) > 0 {
		return strings.TrimSpace(args[0]), nil
	}

	ios := iostreams.FromContext(ctx)

	if !ios.IsStdinTTY() {
		// Read one byte past the limit so an over-long note is reported as
		// such without slurping an unbounded stream.
		b, err := io.ReadAll(io.LimitReader(ios.In, maxNoteLength*utf8.UTFMax+1))
		if err != nil {
			return "", fmt.Errorf("failed to read note from stdin: %w", err)
		}

		return strings.TrimSpace(string(b)), nil
	}

	if !ios.IsInteractive() {
		return "", fmt.Errorf("a feedback note is required: pass it as an argument or through stdin")
	}

	var note string
	if err := prompt.String(ctx, &note, "What would you like to report?", "", true); err != nil {
		return "", err
	}

	return strings.TrimSpace(note), nil
}
