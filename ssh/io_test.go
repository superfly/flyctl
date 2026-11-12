package ssh

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"
)

type testWriteCloser struct {
	io.Writer
}

func (testWriteCloser) Close() error {
	return nil
}

type gatedReader struct {
	ready  <-chan struct{}
	reader io.Reader
}

func (r *gatedReader) Read(p []byte) (int, error) {
	<-r.ready

	return r.reader.Read(p)
}

func TestSessionIOAttachPipesWaitsForOutput(t *testing.T) {
	const expected = "short-lived command output\n"

	outputReady := make(chan struct{})
	runCalled := make(chan struct{})
	result := make(chan error, 1)
	var stdout bytes.Buffer

	sessIO := &SessionIO{
		Stdout: testWriteCloser{Writer: &stdout},
		Stderr: testWriteCloser{Writer: io.Discard},
	}

	go func() {
		result <- sessIO.attachPipes(
			context.Background(),
			testWriteCloser{Writer: io.Discard},
			&gatedReader{ready: outputReady, reader: strings.NewReader(expected)},
			strings.NewReader(""),
			func() error {
				close(runCalled)

				return nil
			},
		)
	}()

	<-runCalled
	select {
	case err := <-result:
		t.Fatalf("attachPipes returned before stdout was drained: %v", err)
	case <-time.After(100 * time.Millisecond):
	}

	close(outputReady)
	select {
	case err := <-result:
		if err != nil {
			t.Fatalf("attachPipes returned an error: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("attachPipes did not return after stdout was drained")
	}

	if got := stdout.String(); got != expected {
		t.Fatalf("stdout = %q, want %q", got, expected)
	}
}

func TestSessionIOAttachPipesHidesRejectedCommand(t *testing.T) {
	sessIO := &SessionIO{}
	cmd := `sqlite3 /data/app.db "SELECT * FROM users WHERE email = 'someone@example.com'"`
	err := sessIO.attachPipes(context.Background(), testWriteCloser{Writer: io.Discard}, strings.NewReader(""), strings.NewReader(""),
		func() error { return fmt.Errorf("ssh: command %v failed", cmd) })
	if !errors.Is(err, ErrExecRejected) || strings.Contains(err.Error(), "sqlite3") {
		t.Fatalf("err = %v, want ErrExecRejected without the command", err)
	}
}

func TestSessionIOForcedCloseIsNotASilentCancel(t *testing.T) {
	ctx, cancel := context.WithCancelCause(context.Background())
	cause := fmt.Errorf("interrupt signal received: %w", context.Canceled) // like signal.NotifyContext
	cancel(cause)
	sessIO := &SessionIO{}
	err := sessIO.attachPipes(ctx, testWriteCloser{Writer: io.Discard}, strings.NewReader(""), strings.NewReader(""),
		func() error { select {} })
	// The CLI prints nothing and exits 127 for context.Canceled; this error
	// must still be shown so the user knows the remote process may be running.
	if errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v must not wrap context.Canceled", err)
	}
	if !strings.Contains(err.Error(), "interrupt signal received") {
		t.Fatalf("err = %v, want the cancel cause in the message", err)
	}
}
