//go:build !windows

package mock

import (
	"bytes"
	"io"
	"strings"

	"github.com/superfly/flyctl/iostreams"
)

// ConfirmationIO answers Survey confirmation prompts without a real terminal.
// Each answer is followed by Enter.
func ConfirmationIO(answers ...string) (*iostreams.IOStreams, *bytes.Buffer, *bytes.Buffer) {
	keys := make([]string, len(answers))
	for i, answer := range answers {
		keys[i] = answer + "\r"
	}

	return PromptIO(keys...)
}

// PromptIO feeds Survey prompts raw keystrokes without a real terminal: one
// argument per prompt, sent exactly as given, so a test can also interrupt a
// prompt or close its input. Survey asks for the screen size and cursor
// position before reading each answer. Invalid file descriptors keep terminal
// ioctls away from real streams.
func PromptIO(keys ...string) (*iostreams.IOStreams, *bytes.Buffer, *bytes.Buffer) {
	var input strings.Builder
	for _, k := range keys {
		input.WriteString("\x1b[24;80R\x1b[1;1R" + k)
	}
	stdout, stderr := new(bytes.Buffer), new(bytes.Buffer)
	ios := &iostreams.IOStreams{
		In:     &promptInput{Reader: strings.NewReader(input.String())},
		Out:    PromptOutput{stdout},
		ErrOut: stderr,
	}
	ios.SetStdinTTY(true)
	ios.SetStdoutTTY(true)

	return ios, stdout, stderr
}

type promptInput struct{ *strings.Reader }

func (r *promptInput) Read(p []byte) (int, error) {
	// Cursor.Location creates a fresh buffered reader for every query; avoid
	// letting that reader consume the next cursor response or answer.
	if len(p) > 1 {
		p = p[:1]
	}
	return r.Reader.Read(p)
}

func (*promptInput) Fd() uintptr  { return ^uintptr(0) }
func (*promptInput) Close() error { return nil }

// PromptOutput is a prompt destination that Survey cannot mistake for a
// terminal, for tests that need the prompt written somewhere other than the
// buffer PromptIO returns.
type PromptOutput struct{ io.Writer }

func (PromptOutput) Fd() uintptr { return ^uintptr(0) }
