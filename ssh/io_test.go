package ssh

import (
	"bytes"
	"context"
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
