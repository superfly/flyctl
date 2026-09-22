package ssh

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"fmt"
	"net"
	"testing"
	"time"

	getsentry "github.com/getsentry/sentry-go"
	pkgerrors "github.com/pkg/errors"
	"github.com/stretchr/testify/require"
	"github.com/superfly/fly-go/flaps"
	gossh "golang.org/x/crypto/ssh"
)

func remoteExit(t *testing.T, status uint32) error {
	t.Helper()
	_, key, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	signer, err := gossh.NewSignerFromKey(key)
	require.NoError(t, err)
	config := &gossh.ServerConfig{NoClientAuth: true}
	config.AddHostKey(signer)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer listener.Close()
	done := make(chan error, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			done <- err
			return
		}
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
		server, channels, requests, err := gossh.NewServerConn(conn, config)
		if err != nil {
			done <- err
			return
		}
		defer server.Close()
		go gossh.DiscardRequests(requests)
		ch, reqs, err := (<-channels).Accept()
		if err != nil {
			done <- err
			return
		}
		defer ch.Close()
		for req := range reqs {
			if req.Type == "exec" {
				_ = req.Reply(true, nil)
				_, err = ch.SendRequest("exit-status", false, gossh.Marshal(struct{ Status uint32 }{status}))
				done <- err
				return
			}
			_ = req.Reply(false, nil)
		}
		done <- fmt.Errorf("no exec request")
	}()
	client, err := gossh.Dial("tcp", listener.Addr().String(), &gossh.ClientConfig{User: "test", HostKeyCallback: gossh.InsecureIgnoreHostKey(), Timeout: 5 * time.Second})
	require.NoError(t, err)
	defer client.Close()
	session, err := client.NewSession()
	require.NoError(t, err)
	defer session.Close()
	err = session.Run("test")
	require.NoError(t, <-done)
	var exit *gossh.ExitError
	require.ErrorAs(t, err, &exit)
	require.Equal(t, int(status), exit.ExitStatus())

	return pkgerrors.Wrap(err, "ssh shell")
}

func TestCaptureSSHError(t *testing.T) {
	var captured int
	client, err := getsentry.NewClient(getsentry.ClientOptions{BeforeSend: func(event *getsentry.Event, _ *getsentry.EventHint) *getsentry.Event { captured++; return nil }})
	require.NoError(t, err)
	hub := getsentry.CurrentHub()
	previous := hub.Client()
	hub.BindClient(client)
	t.Cleanup(func() { hub.BindClient(previous) })
	app := &flaps.App{}
	for _, tc := range []struct {
		name string
		err  error
		want int
	}{
		{"exit 1", remoteExit(t, 1), 0},
		{"exit 2", remoteExit(t, 2), 0},
		{"wrapped cancellation", fmt.Errorf("connect: %w", context.Canceled), 0},
		{"transport", errors.New("connection reset by peer"), 1},
		{"auth", errors.New("ssh: unable to authenticate"), 1},
		{"missing exit status", &gossh.ExitMissingError{}, 1},
		{"matching text is not sufficient", errors.New("ssh shell: Process exited with status 1"), 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			captured = 0
			captureError(context.Background(), tc.err, app)
			require.Equal(t, tc.want, captured)
		})
	}
}
