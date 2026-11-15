package ssh

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/superfly/flyctl/internal/flyerr"
	gossh "golang.org/x/crypto/ssh"
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
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	err := sessIO.attachPipes(ctx, testWriteCloser{Writer: io.Discard}, strings.NewReader(""), strings.NewReader(""),
		func() error { <-release; return nil })
	// The CLI prints nothing and exits 127 for context.Canceled; this error
	// must still be shown so the user knows the remote process may be running.
	if errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v must not wrap context.Canceled", err)
	}
	if !strings.Contains(err.Error(), "interrupt signal received") {
		t.Fatalf("err = %v, want the cancel cause in the message", err)
	}
	// It is marked for Sentry instead, which goes by the mark, not the text.
	if !flyerr.IsExpected(err) {
		t.Fatalf("err = %v, want it marked expected", err)
	}
}

func TestSessionIOForcedCloseByDeadlineIsReported(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()
	sessIO := &SessionIO{}
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	err := sessIO.attachPipes(ctx, testWriteCloser{Writer: io.Discard}, strings.NewReader(""), strings.NewReader(""),
		func() error { <-release; return nil })
	if err == nil || flyerr.IsExpected(err) {
		t.Fatalf("err = %v, want an error that is not marked expected", err)
	}
}

// The rejection is recognised by the wording x/crypto/ssh uses, so run it
// against a real server: if that wording changes, this fails instead of the
// user's command line quietly going back into error reports.
func TestSessionIOHidesCommandRejectedByServer(t *testing.T) {
	_, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := gossh.NewSignerFromKey(key)
	if err != nil {
		t.Fatal(err)
	}
	serverConfig := &gossh.ServerConfig{NoClientAuth: true}
	serverConfig.AddHostKey(signer)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { listener.Close() })

	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()

		_, channels, requests, err := gossh.NewServerConn(conn, serverConfig)
		if err != nil {
			return
		}
		go gossh.DiscardRequests(requests)

		for newChannel := range channels {
			_, channelRequests, err := newChannel.Accept()
			if err != nil {
				return
			}
			go func() {
				for req := range channelRequests {
					_ = req.Reply(false, nil) // refuse exec
				}
			}()
		}
	}()

	client, err := gossh.Dial("tcp", listener.Addr().String(), &gossh.ClientConfig{
		User:            "root",
		HostKeyCallback: gossh.InsecureIgnoreHostKey(),
		Timeout:         5 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { client.Close() })

	sess, err := client.NewSession()
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cmd := `sqlite3 /data/app.db "SELECT * FROM users WHERE email = 'someone@example.com'"`
	err = (&SessionIO{}).attach(ctx, sess, cmd)
	if !errors.Is(err, ErrExecRejected) || strings.Contains(err.Error(), "sqlite3") {
		t.Fatalf("err = %v, want ErrExecRejected without the command", err)
	}
}
