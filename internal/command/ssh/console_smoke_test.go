//go:build windows

package ssh

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
	"io"
	"net"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	flyssh "github.com/superfly/flyctl/ssh"
	gossh "golang.org/x/crypto/ssh"
	"golang.org/x/sys/windows"
)

// Temporary CI proof. Remove after the Windows run, before merging.
func TestConsoleRedirectedSSHSmoke(t *testing.T) {
	for _, output := range []string{"file", "pipe", "closed-input"} {
		for _, status := range []uint32{0, 1, 2} {
			t.Run(fmt.Sprintf("%s/exit-%d", output, status), func(t *testing.T) {
				client := smokeSSHServer(t, status)
				input, err := os.Open(os.DevNull)
				require.NoError(t, err)
				defer input.Close()
				if output == "closed-input" {
					require.NoError(t, input.Close())
				}
				stderr, err := os.CreateTemp(t.TempDir(), "stderr")
				require.NoError(t, err)
				defer stderr.Close()
				var stdout, reader *os.File
				if output == "pipe" {
					reader, stdout, err = os.Pipe()
					require.NoError(t, err)
					defer reader.Close()
				} else {
					stdout, err = os.CreateTemp(t.TempDir(), "stdout")
					require.NoError(t, err)
				}
				defer stdout.Close()
				oldIn, oldOut, oldErr := os.Stdin, os.Stdout, os.Stderr
				os.Stdin, os.Stdout, os.Stderr = input, stdout, stderr
				defer func() { os.Stdin, os.Stdout, os.Stderr = oldIn, oldOut, oldErr }()
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				err = Console(ctx, &flyssh.Client{Client: client}, "smoke", false, SessionTarget{})
				if status == 0 && output == "closed-input" {
					require.ErrorIs(t, err, windows.ERROR_INVALID_HANDLE)
				} else if status == 0 {
					require.NoError(t, err)
				} else {
					var exit *gossh.ExitError
					require.ErrorAs(t, err, &exit)
					require.Equal(t, int(status), exit.ExitStatus())
				}
				var got []byte
				if output == "pipe" {
					require.NoError(t, stdout.Close())
					got, err = io.ReadAll(reader)
				} else {
					_, err = stdout.Seek(0, io.SeekStart)
					require.NoError(t, err)
					got, err = io.ReadAll(stdout)
				}
				require.NoError(t, err)
				require.Equal(t, "stdout proof\n", string(got))
				_, err = stderr.Seek(0, io.SeekStart)
				require.NoError(t, err)
				got, err = io.ReadAll(stderr)
				require.NoError(t, err)
				require.Equal(t, "stderr proof\n", string(got))
			})
		}
	}
}

// The loopback fixture accepts only a fixed command; it never invokes a shell.
func smokeSSHServer(t *testing.T, status uint32) *gossh.Client {
	t.Helper()
	_, key, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	signer, err := gossh.NewSignerFromKey(key)
	require.NoError(t, err)
	config := &gossh.ServerConfig{NoClientAuth: true}
	config.AddHostKey(signer)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = listener.Close() })
	done := make(chan error, 1)
	go func() {
		done <- func() error {
			conn, err := listener.Accept()
			if err != nil {
				return err
			}
			defer conn.Close()
			_ = conn.SetDeadline(time.Now().Add(10 * time.Second))
			server, channels, requests, err := gossh.NewServerConn(conn, config)
			if err != nil {
				return err
			}
			defer server.Close()
			go gossh.DiscardRequests(requests)
			channel, ok := <-channels
			if !ok {
				return fmt.Errorf("session not opened")
			}
			ch, reqs, err := channel.Accept()
			if err != nil {
				return err
			}
			defer ch.Close()
			for req := range reqs {
				if req.Type != "exec" {
					_ = req.Reply(false, nil)
					continue
				}
				var command struct{ Command string }
				if err := gossh.Unmarshal(req.Payload, &command); err != nil {
					return err
				}
				if command.Command != "smoke" {
					return fmt.Errorf("unexpected command")
				}
				if err := req.Reply(true, nil); err != nil {
					return err
				}
				if _, err := io.WriteString(ch, "stdout proof\n"); err != nil {
					return err
				}
				if _, err := io.WriteString(ch.Stderr(), "stderr proof\n"); err != nil {
					return err
				}
				_, err = ch.SendRequest("exit-status", false, gossh.Marshal(struct{ Status uint32 }{status}))

				return err
			}

			return fmt.Errorf("exec not received")
		}()
	}()
	client, err := gossh.Dial("tcp", listener.Addr().String(), &gossh.ClientConfig{
		User: "test", HostKeyCallback: gossh.FixedHostKey(signer.PublicKey()), Timeout: 10 * time.Second,
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = client.Close()
		select {
		case err := <-done:
			require.NoError(t, err)
		case <-time.After(12 * time.Second):
			t.Error("SSH fixture did not stop")
		}
	})

	return client
}
