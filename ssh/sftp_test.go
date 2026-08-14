package ssh

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"io"
	"maps"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

// sftpTestServer runs a stand-in for the SSH server on a machine: it serves
// the SFTP subsystem, and records the env the session carried, which is how a
// client says which container the session should be served from.
//
// It returns a connected Client, the env recorded so far, a channel closed
// once the session the subsystem ran over is done, and a hangup that drops the
// session from the server's end.
func sftpTestServer(t *testing.T) (client *Client, recorded func() map[string]string, sessionDone <-chan struct{}, hangup func()) {
	t.Helper()

	_, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate host key: %v", err)
	}

	signer, err := ssh.NewSignerFromKey(key)
	if err != nil {
		t.Fatalf("sign with host key: %v", err)
	}

	config := &ssh.ServerConfig{NoClientAuth: true}
	config.AddHostKey(signer)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { listener.Close() })

	var (
		mu      sync.Mutex
		env     = map[string]string{}
		done    = make(chan struct{})
		serving = make(chan ssh.Channel, 1)
	)

	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}

		_, chans, reqs, err := ssh.NewServerConn(conn, config)
		if err != nil {
			return
		}

		go ssh.DiscardRequests(reqs)

		for newChan := range chans {
			channel, requests, err := newChan.Accept()
			if err != nil {
				return
			}

			serving <- channel

			go func() {
				defer close(done)
				defer channel.Close()

				for req := range requests {
					switch req.Type {
					case "env":
						var kv struct{ Name, Value string }
						if err := ssh.Unmarshal(req.Payload, &kv); err != nil {
							req.Reply(false, nil)

							continue
						}

						mu.Lock()
						env[kv.Name] = kv.Value
						mu.Unlock()

						req.Reply(true, nil)

					case "subsystem":
						server, err := sftp.NewServer(channel)
						if err != nil {
							req.Reply(false, nil)

							return
						}

						req.Reply(true, nil)
						server.Serve()

						return

					default:
						req.Reply(false, nil)
					}
				}
			}()
		}
	}()

	conn, err := ssh.Dial("tcp", listener.Addr().String(), &ssh.ClientConfig{
		User:            "root",
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	})
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { conn.Close() })

	return &Client{Client: conn}, func() map[string]string {
			mu.Lock()
			defer mu.Unlock()

			return maps.Clone(env)
		}, done, func() {
			(<-serving).Close()
		}
}

// TestSFTPSetsTarget covers what an SFTP session has to carry to be served
// from the right place: the same env a shell session carries, on the session
// the subsystem runs over. Without it the transfers land in the machine's
// namespace no matter which container was selected.
func TestSFTPSetsTarget(t *testing.T) {
	for _, tc := range []struct {
		name   string
		target SessionTarget
		want   map[string]string
	}{
		{
			// Nothing set: the machine decides, as it always has.
			name: "neither",
			want: map[string]string{},
		},
		{
			name:   "container",
			target: SessionTarget{Container: "worker"},
			want:   map[string]string{"FLY_SSH_CONTAINER": "worker"},
		},
		{
			name:   "machine",
			target: SessionTarget{Machine: true},
			want:   map[string]string{"FLY_SSH_MACHINE": "1"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client, recorded, _, _ := sftpTestServer(t)

			ftp, err := client.SFTP(context.Background(), tc.target)
			if err != nil {
				t.Fatalf("open sftp: %v", err)
			}
			defer ftp.Close()

			env := recorded()
			for name, want := range tc.want {
				if env[name] != want {
					t.Errorf("session env %s = %q, want %q", name, env[name], want)
				}
			}

			if len(env) != len(tc.want) {
				t.Errorf("session env is %v, want %v", env, tc.want)
			}

			// The env is only worth anything if the subsystem it was set on is
			// the one carrying the transfers.
			path := filepath.Join(t.TempDir(), "hello")
			if err := os.WriteFile(path, []byte("hello world"), 0o600); err != nil {
				t.Fatalf("write file: %v", err)
			}

			// SFTP paths are POSIX wherever the file is: a local path reaches
			// the server spelled "/C:/..." on Windows, and unchanged anywhere
			// this test would otherwise be running.
			remote := filepath.ToSlash(path)
			if !strings.HasPrefix(remote, "/") {
				remote = "/" + remote
			}

			f, err := ftp.Open(remote)
			if err != nil {
				t.Fatalf("open %s: %v", remote, err)
			}
			defer f.Close()

			content, err := io.ReadAll(f)
			if err != nil {
				t.Fatalf("read %s: %v", remote, err)
			}

			if string(content) != "hello world" {
				t.Errorf("read %q, want %q", content, "hello world")
			}
		})
	}
}

// TestSFTPCloseEndsSession covers the session opened on the caller's behalf:
// nothing else holds a reference to it, so closing the SFTP client has to be
// what closes it.
func TestSFTPCloseEndsSession(t *testing.T) {
	client, _, sessionDone, _ := sftpTestServer(t)

	ftp, err := client.SFTP(context.Background(), SessionTarget{Container: "worker"})
	if err != nil {
		t.Fatalf("open sftp: %v", err)
	}

	if err := ftp.Close(); err != nil {
		t.Fatalf("close sftp: %v", err)
	}

	select {
	case <-sessionDone:
	case <-time.After(10 * time.Second):
		t.Fatal("closing the SFTP client left its session open")
	}
}

// TestSFTPCloseAfterServerHangup covers the other order these end in: the
// server drops the session first -- an early failure on its side, or a
// container server that exits on its own -- and the client closes afterwards.
// The session is already gone by then, and saying so is not an error the
// caller can do anything with.
func TestSFTPCloseAfterServerHangup(t *testing.T) {
	client, _, sessionDone, hangup := sftpTestServer(t)

	ftp, err := client.SFTP(context.Background(), SessionTarget{Container: "worker"})
	if err != nil {
		t.Fatalf("open sftp: %v", err)
	}

	hangup()
	<-sessionDone

	if err := ftp.Close(); err != nil {
		t.Fatalf("close after the server hung up: %v", err)
	}
}
