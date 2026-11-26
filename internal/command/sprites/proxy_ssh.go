package sprites

import (
	"context"
	"crypto/ed25519"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	sprites "github.com/superfly/sprites-go"
	"golang.org/x/crypto/ssh"
)

// proxySSH serves one SSH connection over stdin and stdout, translating SSH
// sessions into commands run in the sprite and direct-tcpip channels into
// connections to the sprite's ports. OpenSSH runs this as a ProxyCommand, so
// ssh, scp, sftp-less rsync and editors' remote modes can reach a sprite
// without it running an SSH server.
//
// Authentication is by the Sprites API token this process already holds, so
// the SSH layer accepts any client key.
func proxySSH(ctx context.Context, s *session, sprite *sprites.Sprite) error {
	return serveSSH(ctx, stdioConn{}, s, sprite)
}

// serveSSH serves one SSH connection arriving on client.
func serveSSH(ctx context.Context, client net.Conn, s *session, sprite *sprites.Sprite) error {
	signer, err := sshHostKey(s, sprite.Name())
	if err != nil {
		return err
	}

	config := &ssh.ServerConfig{
		PublicKeyCallback: func(ssh.ConnMetadata, ssh.PublicKey) (*ssh.Permissions, error) {
			return &ssh.Permissions{}, nil
		},
		NoClientAuth: true,
	}
	config.AddHostKey(signer)

	conn, channels, requests, err := ssh.NewServerConn(client, config)
	if err != nil {
		return fmt.Errorf("failed to establish the SSH connection: %w", err)
	}
	defer conn.Close() //nolint:errcheck

	go ssh.DiscardRequests(requests)

	var wg sync.WaitGroup
	defer wg.Wait()

	for {
		select {
		case <-ctx.Done():
			return nil
		case newChannel, ok := <-channels:
			if !ok {
				return nil
			}

			switch newChannel.ChannelType() {
			case "session":
				wg.Add(1)
				go func() {
					defer wg.Done()
					serveSSHSession(ctx, newChannel, sprite)
				}()
			case "direct-tcpip":
				wg.Add(1)
				go func() {
					defer wg.Done()
					serveSSHForward(ctx, newChannel, sprite)
				}()
			default:
				_ = newChannel.Reject(ssh.UnknownChannelType, "unsupported channel type")
			}
		}
	}
}

// sshHostKey derives a stable host key for a sprite, so OpenSSH's known_hosts
// check passes on later connections. The key is not a secret: the transport is
// already authenticated and encrypted by the Sprites API connection.
func sshHostKey(s *session, sprite string) (ssh.Signer, error) {
	mac := hmac.New(sha256.New, []byte("fly-sprites-proxy-ssh-host-key-v1"))
	for _, part := range []string{s.APIURL, s.Org, sprite} {
		mac.Write([]byte(part))
		mac.Write([]byte{0})
	}

	return ssh.NewSignerFromKey(ed25519.NewKeyFromSeed(mac.Sum(nil)))
}

// SSH request payloads, as defined by RFC 4254.
type (
	sshEnvRequest struct {
		Name  string
		Value string
	}

	sshExecRequest struct {
		Command string
	}

	sshPtyRequest struct {
		Term   string
		Cols   uint32
		Rows   uint32
		Width  uint32
		Height uint32
		Modes  string
	}

	sshWindowChangeRequest struct {
		Cols   uint32
		Rows   uint32
		Width  uint32
		Height uint32
	}

	sshSignalRequest struct {
		Signal string
	}

	sshDirectTCPIPRequest struct {
		DestHost   string
		DestPort   uint32
		OriginHost string
		OriginPort uint32
	}
)

// sshSession is the state of one SSH session channel.
type sshSession struct {
	sprite  *sprites.Sprite
	channel ssh.Channel

	mu         sync.Mutex
	env        []string
	tty        bool
	rows, cols uint16
	cmd        *sprites.Cmd
	started    bool
}

func serveSSHSession(ctx context.Context, newChannel ssh.NewChannel, sprite *sprites.Sprite) {
	channel, requests, err := newChannel.Accept()
	if err != nil {
		return
	}
	defer channel.Close() //nolint:errcheck

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	sess := &sshSession{sprite: sprite, channel: channel}

	for {
		select {
		case <-ctx.Done():
			return
		case req, ok := <-requests:
			if !ok {
				return
			}

			err := sess.handle(ctx, cancel, req)
			if req.WantReply {
				_ = req.Reply(err == nil, nil)
			}
		}
	}
}

// handle applies one session request. The session ends, through done, when
// the command it starts finishes.
func (s *sshSession) handle(ctx context.Context, done context.CancelFunc, req *ssh.Request) error {
	switch req.Type {
	case "env":
		var payload sshEnvRequest
		if err := ssh.Unmarshal(req.Payload, &payload); err != nil {
			return err
		}

		s.mu.Lock()
		s.env = append(s.env, payload.Name+"="+payload.Value)
		s.mu.Unlock()

		return nil
	case "pty-req":
		var payload sshPtyRequest
		if err := ssh.Unmarshal(req.Payload, &payload); err != nil {
			return err
		}

		s.mu.Lock()
		defer s.mu.Unlock()

		s.tty = true
		s.rows, s.cols = uint16(payload.Rows), uint16(payload.Cols)

		if payload.Term != "" {
			s.env = append(s.env, "TERM="+payload.Term)
		}

		return nil
	case "window-change":
		var payload sshWindowChangeRequest
		if err := ssh.Unmarshal(req.Payload, &payload); err != nil {
			return err
		}

		s.mu.Lock()
		s.rows, s.cols = uint16(payload.Rows), uint16(payload.Cols)
		cmd := s.cmd
		s.mu.Unlock()

		if cmd != nil {
			return cmd.SetTTYSize(uint16(payload.Rows), uint16(payload.Cols))
		}

		return nil
	case "signal":
		var payload sshSignalRequest
		if err := ssh.Unmarshal(req.Payload, &payload); err != nil {
			return err
		}

		s.mu.Lock()
		cmd := s.cmd
		s.mu.Unlock()

		if cmd == nil {
			return errors.New("no running command to signal")
		}

		return cmd.Signal(payload.Signal)
	case "shell":
		return s.start(ctx, done, "")
	case "exec":
		var payload sshExecRequest
		if err := ssh.Unmarshal(req.Payload, &payload); err != nil {
			return err
		}

		return s.start(ctx, done, payload.Command)
	default:
		return fmt.Errorf("unsupported request type %q", req.Type)
	}
}

// start runs command, or a login shell when it is empty, as the sprite user
// from their home directory, the way an SSH server would.
func (s *sshSession) start(ctx context.Context, done context.CancelFunc, command string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.started {
		return errors.New("the session already has a command running")
	}
	s.started = true

	if command == "" {
		command = consoleCommand
	}

	args := []string{"--user=sprite", "--non-interactive", "--set-home"}
	if names := envNames(s.env); names != "" {
		args = append(args, "--preserve-env="+names)
	}

	args = append(args, "--", "/bin/sh", "-c", `cd && exec "${SHELL:-/bin/bash}" -lc "$1"`, "sh", command)

	cmd := s.sprite.CommandContext(ctx, "/usr/bin/sudo", args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = s.channel, s.channel, s.channel.Stderr()
	cmd.Env = s.env

	if s.tty {
		cmd.SetTTY(true)

		if s.rows > 0 && s.cols > 0 {
			_ = cmd.SetTTYSize(s.rows, s.cols)
		}
	}

	if err := cmd.Start(); err != nil {
		fmt.Fprintf(s.channel.Stderr(), "failed to start command in sprite: %v\r\n", err)
		return err
	}

	s.cmd = cmd

	// The size given before the start is not applied to the new terminal, so
	// send it again now that the command is running.
	if s.tty && s.rows > 0 && s.cols > 0 {
		_ = cmd.SetTTYSize(s.rows, s.cols)
	}

	go func() {
		defer done()

		// A command that could not be waited on still has to end the SSH
		// session with a failing status.
		status := uint32(255)

		var exitErr *sprites.ExitError

		switch err := cmd.Wait(); {
		case err == nil:
			status = 0
		case errors.As(err, &exitErr):
			status = uint32(exitErr.ExitCode())
		}

		var payload [4]byte
		binary.BigEndian.PutUint32(payload[:], status)

		_, _ = s.channel.SendRequest("exit-status", false, payload[:])
	}()

	return nil
}

// envNames lists the variable names in env, for sudo's --preserve-env.
func envNames(env []string) string {
	names := make([]string, 0, len(env))

	for _, pair := range env {
		if name, _, ok := strings.Cut(pair, "="); ok && name != "" {
			names = append(names, name)
		}
	}

	return strings.Join(names, ",")
}

// serveSSHForward connects a direct-tcpip channel, which is what `ssh -L` and
// `ssh -W` open, to a port in the sprite.
func serveSSHForward(ctx context.Context, newChannel ssh.NewChannel, sprite *sprites.Sprite) {
	var payload sshDirectTCPIPRequest
	if err := ssh.Unmarshal(newChannel.ExtraData(), &payload); err != nil {
		_ = newChannel.Reject(ssh.ConnectionFailed, "invalid direct-tcpip request")
		return
	}

	addr := net.JoinHostPort(payload.DestHost, strconv.FormatUint(uint64(payload.DestPort), 10))

	remote, err := sprite.ProxySocket(ctx, "tcp", addr)
	if err != nil {
		_ = newChannel.Reject(ssh.ConnectionFailed, err.Error())
		return
	}
	defer remote.Close() //nolint:errcheck

	channel, requests, err := newChannel.Accept()
	if err != nil {
		return
	}
	defer channel.Close() //nolint:errcheck

	go ssh.DiscardRequests(requests)

	stop := context.AfterFunc(ctx, func() {
		_ = channel.Close()
		_ = remote.Close()
	})
	defer stop()

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		_, _ = io.Copy(remote, channel)
		_ = remote.Close()
	}()

	go func() {
		defer wg.Done()
		_, _ = io.Copy(channel, remote)
		_ = channel.Close()
	}()

	wg.Wait()
}

// stdioConn presents the process's stdin and stdout as a net.Conn, which is
// what the SSH server reads the client's connection from.
type stdioConn struct{}

func (stdioConn) Read(p []byte) (int, error)  { return os.Stdin.Read(p) }
func (stdioConn) Write(p []byte) (int, error) { return os.Stdout.Write(p) }

func (stdioConn) Close() error {
	return errors.Join(os.Stdin.Close(), os.Stdout.Close())
}

func (stdioConn) LocalAddr() net.Addr              { return stdioAddr{} }
func (stdioConn) RemoteAddr() net.Addr             { return stdioAddr{} }
func (stdioConn) SetDeadline(time.Time) error      { return nil }
func (stdioConn) SetReadDeadline(time.Time) error  { return nil }
func (stdioConn) SetWriteDeadline(time.Time) error { return nil }

type stdioAddr struct{}

func (stdioAddr) Network() string { return "stdio" }
func (stdioAddr) String() string  { return "stdio" }
