//go:build !windows

package mock

import (
	"bytes"
	"strings"

	"github.com/superfly/flyctl/iostreams"
)

// ConfirmationIO answers Survey confirmation prompts without a real terminal.
// Survey asks for the screen size and cursor position before reading each
// answer. Invalid file descriptors keep terminal ioctls away from real streams.
func ConfirmationIO(answers ...string) (*iostreams.IOStreams, *bytes.Buffer, *bytes.Buffer) {
	var input strings.Builder
	for _, answer := range answers {
		input.WriteString("\x1b[24;80R\x1b[1;1R" + answer + "\r")
	}
	stdout, stderr := new(bytes.Buffer), new(bytes.Buffer)
	ios := &iostreams.IOStreams{
		In:     &confirmationReader{Reader: strings.NewReader(input.String())},
		Out:    confirmationWriter{stdout},
		ErrOut: stderr,
	}
	ios.SetStdinTTY(true)
	ios.SetStdoutTTY(true)

	return ios, stdout, stderr
}

type confirmationReader struct{ *strings.Reader }

func (r *confirmationReader) Read(p []byte) (int, error) {
	// Cursor.Location creates a fresh buffered reader for every query; avoid
	// letting that reader consume the next cursor response or answer.
	if len(p) > 1 {
		p = p[:1]
	}
	return r.Reader.Read(p)
}

func (*confirmationReader) Fd() uintptr  { return ^uintptr(0) }
func (*confirmationReader) Close() error { return nil }

type confirmationWriter struct{ *bytes.Buffer }

func (confirmationWriter) Fd() uintptr { return ^uintptr(0) }
