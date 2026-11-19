package sprites

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/superfly/flyctl/internal/flyerr"
)

// exitCode returns the exit status err asks flyctl to end with.
func exitCode(t *testing.T, err error) int {
	t.Helper()

	var exitErr *flyerr.ExitCodeError
	require.ErrorAs(t, err, &exitErr)

	return exitErr.Code
}

func fastRetries(t *testing.T) {
	t.Helper()

	original := startRetryDelay
	startRetryDelay = time.Millisecond

	t.Cleanup(func() { startRetryDelay = original })
}

func TestCheckpointCreate(t *testing.T) {
	api := newSpritesAPI(t)
	api.addSprite("dev")

	e := newEnv(t, newCheckpointCreate(), "--sprite", "dev", "--comment", "before upgrade")
	require.NoError(t, runCheckpointCreate(e.ctx, api.session()))

	assert.Contains(t, e.out.String(), "Checkpoint v1 created")
	assert.Contains(t, e.out.String(), "fly sprites restore v1")
	assert.Equal(t, "before upgrade", api.sprites["dev"].checkpoints[0]["comment"])
}

func TestCheckpointCreateUnknownSprite(t *testing.T) {
	api := newSpritesAPI(t)

	e := newEnv(t, newCheckpointCreate(), "--sprite", "ghost")
	require.ErrorContains(t, runCheckpointCreate(e.ctx, api.session()), "failed to create checkpoint")
}

func TestCheckpointList(t *testing.T) {
	api := newSpritesAPI(t)
	sp := api.addSprite("dev")

	e := newEnv(t, newCheckpointList(), "--sprite", "dev")
	require.NoError(t, runCheckpointList(e.ctx, api.session()))
	assert.Equal(t, "No checkpoints found.\n", e.out.String())

	e = newEnv(t, newCheckpointList(), "--sprite", "dev", "--json")
	require.NoError(t, runCheckpointList(e.ctx, api.session()))
	assert.JSONEq(t, "[]", e.out.String())

	created := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	sp.checkpoints = []map[string]any{
		{"id": "Current", "create_time": created},
		{"id": "v2", "create_time": created, "comment": "nightly", "is_auto": true},
		{"id": "v1", "create_time": created, "comment": "first"},
	}

	e = newEnv(t, newCheckpointList(), "--sprite", "dev", "--include-auto", "--history", "v1")
	require.NoError(t, runCheckpointList(e.ctx, api.session()))

	out := e.out.String()
	assert.Contains(t, out, "Current (active)")
	assert.Contains(t, out, "v2 (auto)")
	assert.Contains(t, out, "2026-01-02 03:04:05")
	assert.Contains(t, out, "first")

	e = newEnv(t, newCheckpointList(), "--sprite", "dev", "--json")
	require.NoError(t, runCheckpointList(e.ctx, api.session()))

	var listed []map[string]any
	require.NoError(t, json.Unmarshal(e.out.Bytes(), &listed))
	require.Len(t, listed, 3)
	assert.Equal(t, "v2", listed[1]["id"])
}

func TestCheckpointInfo(t *testing.T) {
	api := newSpritesAPI(t)
	api.addSprite("dev").checkpoints = []map[string]any{
		{"id": "v2", "create_time": time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC), "comment": "second", "history": []string{"v1"}},
		{"id": "v1", "create_time": time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)},
	}

	e := newEnv(t, newCheckpointInfo(), "--sprite", "dev", "v2")
	require.NoError(t, runCheckpointInfo(e.ctx, api.session()))
	assert.Equal(t, "ID: v2\nCreated: 2026-01-02T03:04:05Z\nComment: second\nHistory:\n  v1\n", e.out.String())

	e = newEnv(t, newCheckpointInfo(), "--sprite", "dev", "v1")
	require.NoError(t, runCheckpointInfo(e.ctx, api.session()))
	assert.Equal(t, "ID: v1\nCreated: 2026-01-01T00:00:00Z\nHistory: (none)\n", e.out.String())

	e = newEnv(t, newCheckpointInfo(), "--sprite", "dev", "--json", "v2")
	require.NoError(t, runCheckpointInfo(e.ctx, api.session()))
	assert.Contains(t, e.out.String(), `"comment": "second"`)

	e = newEnv(t, newCheckpointInfo(), "--sprite", "dev", "v9")
	require.ErrorContains(t, runCheckpointInfo(e.ctx, api.session()), "failed to get checkpoint info")
}

func TestRestore(t *testing.T) {
	api := newSpritesAPI(t)
	api.addSprite("dev").checkpoints = []map[string]any{{"id": "v1"}}

	// Both spellings of the command run the same thing.
	for _, cmd := range []*cobra.Command{newRestore(), newCheckpointRestore()} {
		e := newEnv(t, cmd, "--sprite", "dev", "v1")
		require.NoError(t, runRestore(e.ctx, api.session()))

		assert.Contains(t, e.out.String(), "Restored to checkpoint v1")
		assert.Contains(t, e.errOut.String(), "Warning: active sessions were ended")
	}

	e := newEnv(t, newRestore(), "--sprite", "dev", "v9")
	require.ErrorContains(t, runRestore(e.ctx, api.session()), "failed to restore checkpoint")
}

func TestSessionsList(t *testing.T) {
	api := newSpritesAPI(t)
	sp := api.addSprite("dev")

	e := newEnv(t, newSessionsList(), "--sprite", "dev")
	require.NoError(t, runSessionsList(e.ctx, api.session()))
	assert.Equal(t, "No running sessions.\n", e.out.String())

	e = newEnv(t, newSessionsList(), "--sprite", "dev", "--json")
	require.NoError(t, runSessionsList(e.ctx, api.session()))
	assert.JSONEq(t, "[]", e.out.String())

	sp.sessions = []map[string]any{
		{"id": "3", "command": "bash -l", "tty": true, "is_active": true, "created": time.Now().Add(-5 * time.Minute).Format(time.RFC3339)},
		{"id": "7", "command": "npm start", "tty": false, "created": time.Now().Add(-3 * time.Hour).Format(time.RFC3339)},
	}

	e = newEnv(t, newSessionsList(), "--sprite", "dev")
	require.NoError(t, runSessionsList(e.ctx, api.session()))

	out := e.out.String()
	assert.Regexp(t, `3\s+│\s+bash -l\s+│\s+yes\s+│\s+5m ago\s+│\s+active`, out)
	assert.Regexp(t, `7\s+│\s+npm start\s+│\s+no\s+│\s+3h ago\s+│\s+idle`, out)

	e = newEnv(t, newSessionsList(), "--sprite", "dev", "--json")
	require.NoError(t, runSessionsList(e.ctx, api.session()))
	assert.Contains(t, e.out.String(), `"command": "npm start"`)
}

func TestExecRelaysOutputAndExitStatus(t *testing.T) {
	api := newSpritesAPI(t)
	api.addSprite("dev")

	api.exec = func(c *execConn) {
		c.stdout("out\n")
		c.stderr("err\n")
		c.exit(7)
	}

	e := newEnv(t, newExec(), "--sprite", "dev", "--dir", "/app", "--env", "A=1,B=2", "--no-stdin", "--", "sh", "-c", "exit 7")
	err := runExec(e.ctx, api.session())

	assert.Equal(t, 7, exitCode(t, err))
	assert.Equal(t, "out\n", e.out.String())
	assert.Equal(t, "err\n", e.errOut.String())

	query := api.lastExecQuery()
	assert.Equal(t, []string{"sh", "-c", "exit 7"}, query["cmd"])
	assert.Equal(t, "/app", query.Get("dir"))
	assert.Equal(t, []string{"A=1", "B=2"}, query["env"])
	assert.Equal(t, "false", query.Get("stdin"))
	assert.Empty(t, query.Get("tty"))
}

func TestExecSucceeds(t *testing.T) {
	api := newSpritesAPI(t)
	api.addSprite("dev")

	e := newEnv(t, newExec(), "--sprite", "dev", "--no-stdin", "true")
	require.NoError(t, runExec(e.ctx, api.session()))
}

func TestExecForwardsPipedStdin(t *testing.T) {
	api := newSpritesAPI(t)
	api.addSprite("dev")

	api.exec = func(c *execConn) {
		c.stdout("got:" + c.readStdin())
		c.exit(0)
	}

	// The test streams' stdin is not a terminal, so it counts as piped.
	e := newEnv(t, newExec(), "--sprite", "dev", "cat")
	e.in.WriteString("hello")

	require.NoError(t, runExec(e.ctx, api.session()))
	assert.Equal(t, "got:hello", e.out.String())
	assert.Equal(t, "true", api.lastExecQuery().Get("stdin"))
}

func TestExecUploadsFilesFirst(t *testing.T) {
	api := newSpritesAPI(t)
	sp := api.addSprite("dev")

	dir := t.TempDir()
	script := filepath.Join(dir, "run.sh")
	require.NoError(t, os.WriteFile(script, []byte("#!/bin/sh\n"), 0o755))

	var uploadedBeforeExec bool
	api.exec = func(c *execConn) {
		api.mu.Lock()
		uploadedBeforeExec = sp.files[remoteHome+"/run.sh"] != nil
		api.mu.Unlock()

		c.exit(0)
	}

	e := newEnv(t, newExec(), "--sprite", "dev", "--no-stdin", "--file", script+":run.sh", "--file", script+":/opt/abs.sh", "./run.sh")
	require.NoError(t, runExec(e.ctx, api.session()))

	assert.True(t, uploadedBeforeExec, "files are uploaded before the command starts")

	// A relative destination lands in the home directory, an absolute one
	// where it says.
	require.Contains(t, sp.files, remoteHome+"/run.sh")
	require.Contains(t, sp.files, "/opt/abs.sh")
	assert.Equal(t, "#!/bin/sh\n", string(sp.files["/opt/abs.sh"].data))

	if runtime.GOOS != "windows" {
		assert.Equal(t, "755", sp.files[remoteHome+"/run.sh"].mode, "the local permissions are kept")
	}

	assert.Contains(t, e.errOut.String(), "Uploaded "+script)
}

func TestExecFileUploadFailureStopsCommand(t *testing.T) {
	api := newSpritesAPI(t)
	api.addSprite("dev")

	e := newEnv(t, newExec(), "--sprite", "dev", "--no-stdin", "--file", "/does/not/exist:x", "true")
	require.ErrorContains(t, runExec(e.ctx, api.session()), "failed to read /does/not/exist")
	assert.Nil(t, api.lastExecQuery(), "the command must not run when an upload fails")
}

func TestExecRetriesWhenSpriteIsUnavailable(t *testing.T) {
	fastRetries(t)

	api := newSpritesAPI(t)
	api.addSprite("dev")

	api.failExec, api.failExecStatus = 2, http.StatusServiceUnavailable
	api.exec = func(c *execConn) {
		c.stdout("finally\n")
		c.exit(0)
	}

	e := newEnv(t, newExec(), "--sprite", "dev", "--no-stdin", "true")
	require.NoError(t, runExec(e.ctx, api.session()))

	assert.Equal(t, "finally\n", e.out.String())
	assert.Equal(t, 2, strings.Count(e.errOut.String(), "retrying in"))
	assert.Len(t, api.execQueries, 3)
}

func TestExecGivesUpAfterMaxAttempts(t *testing.T) {
	fastRetries(t)
	t.Setenv(startAttemptsEnv, "2")

	api := newSpritesAPI(t)
	api.addSprite("dev")
	api.failExec, api.failExecStatus = 10, http.StatusBadGateway

	e := newEnv(t, newExec(), "--sprite", "dev", "--no-stdin", "true")
	require.ErrorContains(t, runExec(e.ctx, api.session()), "failed to start command")
	assert.Len(t, api.execQueries, 2)
}

func TestExecDoesNotRetryRejectedRequests(t *testing.T) {
	fastRetries(t)

	api := newSpritesAPI(t)
	api.addSprite("dev")
	api.failExec, api.failExecStatus = 10, http.StatusForbidden

	e := newEnv(t, newExec(), "--sprite", "dev", "--no-stdin", "true")
	require.ErrorContains(t, runExec(e.ctx, api.session()), "failed to start command")
	assert.Len(t, api.execQueries, 1)
}

func TestStartAttempts(t *testing.T) {
	t.Setenv(startAttemptsEnv, "")
	assert.Equal(t, defaultStartAttempts, startAttempts())

	t.Setenv(startAttemptsEnv, "9")
	assert.Equal(t, 9, startAttempts())

	for _, bad := range []string{"0", "-1", "many"} {
		t.Setenv(startAttemptsEnv, bad)
		assert.Equal(t, defaultStartAttempts, startAttempts(), bad)
	}
}

func TestExecInterruptSignalsRemoteCommand(t *testing.T) {
	api := newSpritesAPI(t)
	api.addSprite("dev")

	started := make(chan struct{})
	var signal atomic.Value

	api.exec = func(c *execConn) {
		close(started)

		if msg := c.waitControl("signal"); msg != nil {
			signal.Store(msg["signal"])
		}

		c.exit(130)
	}

	e := newEnv(t, newExec(), "--sprite", "dev", "--no-stdin", "sleep", "600")

	ctx, cancel := context.WithCancel(e.ctx)
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- runExec(ctx, api.session()) }()

	<-started
	cancel() // what Ctrl+C does

	select {
	case err := <-done:
		assert.Equal(t, 130, exitCode(t, err))
	case <-time.After(10 * time.Second):
		t.Fatal("exec did not return after an interrupt")
	}

	assert.Equal(t, "INT", signal.Load(), "the remote command is asked to stop, not just abandoned")
}

func TestExecInterruptReportsRemoteExitStatus(t *testing.T) {
	for _, code := range []int{0, 2} {
		t.Run(strconv.Itoa(code), func(t *testing.T) {
			api := newSpritesAPI(t)
			api.addSprite("dev")

			started := make(chan struct{})

			// The remote command traps SIGINT and shuts down on its own terms.
			api.exec = func(c *execConn) {
				close(started)
				c.waitControl("signal")
				c.exit(code)
			}

			e := newEnv(t, newExec(), "--sprite", "dev", "--no-stdin", "server")

			ctx, cancel := context.WithCancel(e.ctx)
			defer cancel()

			done := make(chan error, 1)
			go func() { done <- runExec(ctx, api.session()) }()

			<-started
			cancel()

			select {
			case err := <-done:
				if code == 0 {
					require.NoError(t, err)
				} else {
					assert.Equal(t, code, exitCode(t, err))
				}
			case <-time.After(10 * time.Second):
				t.Fatal("exec did not return after an interrupt")
			}

			assert.Contains(t, e.errOut.String(), "Sent INT to the remote command")
		})
	}
}

func TestExecDroppedConnectionIsNotAnExitStatus(t *testing.T) {
	for _, tty := range []bool{false, true} {
		t.Run(fmt.Sprintf("tty=%v", tty), func(t *testing.T) {
			api := newSpritesAPI(t)
			api.addSprite("dev")

			// The connection ends without the command having exited.
			api.exec = func(c *execConn) { c.stdout("partial") }

			args := []string{"--sprite", "dev", "--no-stdin", "server"}
			if tty {
				args = []string{"--sprite", "dev", "--tty", "server"}
			}

			e := newEnv(t, newExec(), args...)
			err := runExec(e.ctx, api.session())
			require.Error(t, err)

			// Reporting this as the command's own exit status would pass a
			// lost connection off as the command failing.
			var exitErr *flyerr.ExitCodeError
			assert.False(t, errors.As(err, &exitErr), "got exit status from a dropped connection: %v", err)
		})
	}
}

func TestRemoteSignalName(t *testing.T) {
	assert.Equal(t, "INT", remoteSignalName(nil))
	assert.Equal(t, "INT", remoteSignalName(context.Canceled))
	assert.Equal(t, "INT", remoteSignalName(signalCause(os.Interrupt)))
	assert.Equal(t, "TERM", remoteSignalName(signalCause(syscall.SIGTERM)))
}

func TestExecInterruptedBeforeStart(t *testing.T) {
	api := newSpritesAPI(t)
	api.addSprite("dev")

	e := newEnv(t, newExec(), "--sprite", "dev", "--no-stdin", "true")

	ctx, cancel := context.WithCancel(e.ctx)
	cancel()

	var signalled atomic.Bool
	api.exec = func(c *execConn) {
		if c.waitControl("signal") != nil {
			signalled.Store(true)
		}

		c.exit(130)
	}

	err := runRemote(ctx, api.session().Client.Sprite("dev"), remoteOptions{args: []string{"true"}})
	assert.Equal(t, 130, exitCode(t, err))

	// The interrupt arrived while connecting, but the command had been
	// dispatched, so it is told to stop rather than left running.
	assert.True(t, signalled.Load())
}

func TestExecForwardsOpenedPorts(t *testing.T) {
	api := newSpritesAPI(t)
	api.addSprite("dev")

	// A port that is free locally, which the "remote" command then opens.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	port := listener.Addr().(*net.TCPAddr).Port
	require.NoError(t, listener.Close())

	api.tcp = func(remotePort int, conn io.ReadWriter) {
		fmt.Fprintf(conn, "hello from port %d", remotePort)
	}

	release := make(chan struct{})
	api.exec = func(c *execConn) {
		c.text(map[string]any{"type": "port_opened", "port": port, "address": "127.0.0.1", "pid": 42})
		<-release
		c.text(map[string]any{"type": "port_closed", "port": port, "address": "127.0.0.1", "pid": 42})
		c.exit(0)
	}

	e := newEnv(t, newExec(), "--sprite", "dev", "--no-stdin", "server")

	done := make(chan error, 1)
	go func() { done <- runExec(e.ctx, api.session()) }()

	// The same port on localhost now reaches the sprite.
	var conn net.Conn
	require.Eventually(t, func() bool {
		conn, err = net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", port))
		return err == nil
	}, 10*time.Second, 20*time.Millisecond)

	reply, err := io.ReadAll(conn)
	require.NoError(t, err)
	assert.Equal(t, fmt.Sprintf("hello from port %d", port), string(reply))

	close(release)
	require.NoError(t, <-done)

	// Once the command is done the forward is gone.
	_, err = net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), time.Second)
	require.Error(t, err)
}

func TestExecNoPortForward(t *testing.T) {
	api := newSpritesAPI(t)
	api.addSprite("dev")

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	port := listener.Addr().(*net.TCPAddr).Port
	require.NoError(t, listener.Close())

	release := make(chan struct{})
	api.exec = func(c *execConn) {
		c.text(map[string]any{"type": "port_opened", "port": port, "address": "127.0.0.1"})
		<-release
		c.exit(0)
	}

	e := newEnv(t, newExec(), "--sprite", "dev", "--no-stdin", "--no-port-forward", "server")

	done := make(chan error, 1)
	go func() { done <- runExec(e.ctx, api.session()) }()

	time.Sleep(200 * time.Millisecond)

	_, err = net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 200*time.Millisecond)
	require.Error(t, err, "nothing should be listening with --no-port-forward")

	close(release)
	require.NoError(t, <-done)
}

func TestExecTTYWithoutTerminal(t *testing.T) {
	api := newSpritesAPI(t)
	api.addSprite("dev")

	t.Setenv("TERM", "xterm-256color")
	t.Setenv("COLORTERM", "")
	t.Setenv("LANG", "")
	t.Setenv("LC_ALL", "")

	api.exec = func(c *execConn) {
		c.stdout("raw tty output")
		c.exit(4)
	}

	// --tty from a script: a pty is still asked for, but there is no local
	// terminal to put in raw mode or to detach from.
	e := newEnv(t, newExec(), "--sprite", "dev", "--tty", "top")
	err := runExec(e.ctx, api.session())

	assert.Equal(t, 4, exitCode(t, err))
	assert.Equal(t, "raw tty output", e.out.String())

	query := api.lastExecQuery()
	assert.Equal(t, "true", query.Get("tty"))
	assert.Equal(t, []string{"TERM=xterm-256color"}, query["env"])
}

func TestConsole(t *testing.T) {
	api := newSpritesAPI(t)
	sp := api.addSprite("dev")

	// A terminal the sprite image does not know, with an entry on disk.
	terminfo := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(terminfo, "x"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(terminfo, "x", "xterm-fancy"), []byte("compiled-entry"), 0o644))

	t.Setenv("TERM", "xterm-fancy")
	t.Setenv("TERMINFO", terminfo)
	t.Setenv("SHELL", "/bin/zsh")

	for _, name := range []string{"BASH_VERSION", "ZSH_VERSION", "FISH_VERSION", "KSH_VERSION", "tcsh", "COLORTERM", "LANG", "LC_ALL"} {
		t.Setenv(name, "")
	}

	e := newEnv(t, newConsole(), "--sprite", "dev")
	require.NoError(t, runConsole(e.ctx, api.session()))

	query := api.lastExecQuery()
	assert.Equal(t, []string{consoleCommand}, query["cmd"])
	assert.Equal(t, "true", query.Get("tty"))
	assert.ElementsMatch(t, []string{"SHELL=/bin/zsh", "TERM=xterm-fancy"}, query["env"])

	// The terminal's description was installed in the sprite.
	require.Contains(t, sp.files, "/usr/share/terminfo/x/xterm-fancy")
	assert.Equal(t, "compiled-entry", string(sp.files["/usr/share/terminfo/x/xterm-fancy"].data))
}

func TestShareTerminfo(t *testing.T) {
	api := newSpritesAPI(t)
	sp := api.addSprite("dev")
	sprite := api.session().Client.Sprite("dev")

	terminfo := t.TempDir()
	t.Setenv("TERMINFO", terminfo)
	t.Setenv("TERMINFO_DIRS", "")
	t.Setenv("HOME", t.TempDir())

	// Nothing to send for a terminal every image knows, or for none at all.
	for _, term := range []string{"", "xterm-256color", "../escape"} {
		t.Setenv("TERM", term)
		assert.Nil(t, shareTerminfo(context.Background(), sprite), term)
	}

	assert.Empty(t, sp.files)

	// macOS files entries under the first character's hex code.
	require.NoError(t, os.MkdirAll(filepath.Join(terminfo, "67"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(terminfo, "67", "ghostly"), []byte("entry"), 0o644))

	t.Setenv("TERM", "ghostly")
	assert.Nil(t, shareTerminfo(context.Background(), sprite))
	assert.Equal(t, "entry", string(sp.files["/usr/share/terminfo/g/ghostly"].data))

	// When the entry cannot be installed it travels in the environment.
	env := shareTerminfo(context.Background(), sessionWithToken(api, "wrong").Client.Sprite("dev"))
	assert.Equal(t, []string{terminfoEnv + "=ZW50cnk="}, env)
}

func TestLocalTerminfoUnknownTerminal(t *testing.T) {
	t.Setenv("TERMINFO", t.TempDir())
	t.Setenv("TERMINFO_DIRS", t.TempDir())
	t.Setenv("HOME", t.TempDir())

	// No file anywhere, and infocmp does not know it either.
	assert.Nil(t, localTerminfo(context.Background(), "no-such-terminal-zzz"))
}

func TestAttachToNonTTYSession(t *testing.T) {
	api := newSpritesAPI(t)
	sp := api.addSprite("dev")
	sp.sessions = []map[string]any{{"id": "7", "command": "npm start", "tty": false}}

	api.exec = func(c *execConn) {
		c.stdout("earlier output\n")
		c.exit(0)
	}

	e := newEnv(t, newSessionsAttach(), "--sprite", "dev", "7")
	require.NoError(t, runSessionsAttach(e.ctx, api.session()))

	assert.Equal(t, "earlier output\n", e.out.String())

	query := api.lastExecQuery()
	assert.Equal(t, "7", query.Get("id"))
	assert.Equal(t, "0", query.Get("output_offset"), "a session without a screen is replayed from the start")
	assert.Equal(t, "true", query.Get("stdin"), "watching must not end the session's input")
	assert.Empty(t, query["cmd"])
}

func TestAttachInterruptLeavesSessionAlone(t *testing.T) {
	api := newSpritesAPI(t)
	api.addSprite("dev").sessions = []map[string]any{{"id": "7", "command": "npm start", "tty": false}}

	started := make(chan struct{})
	var signalled atomic.Bool

	api.exec = func(c *execConn) {
		close(started)

		for msg := range c.messages {
			if msg.control != nil && msg.control["type"] == "signal" {
				signalled.Store(true)
			}
		}
	}

	e := newEnv(t, newSessionsAttach(), "--sprite", "dev", "7")

	ctx, cancel := context.WithCancel(e.ctx)
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- runSessionsAttach(ctx, api.session()) }()

	<-started
	cancel()

	select {
	case err := <-done:
		assert.Equal(t, 130, exitCode(t, err))
	case <-time.After(10 * time.Second):
		t.Fatal("attach did not return after an interrupt")
	}

	time.Sleep(100 * time.Millisecond)
	assert.False(t, signalled.Load(), "Ctrl+C while watching must not interrupt someone else's session")
}

func TestSessionsAttachUnknownID(t *testing.T) {
	api := newSpritesAPI(t)
	api.addSprite("dev").sessions = []map[string]any{{"id": "7", "command": "npm start"}}

	e := newEnv(t, newSessionsAttach(), "--sprite", "dev", "99")
	require.ErrorContains(t, runSessionsAttach(e.ctx, api.session()), "no session with ID 99")
	assert.Nil(t, api.lastExecQuery())
}

func TestAttachByCommand(t *testing.T) {
	api := newSpritesAPI(t)
	api.addSprite("dev").sessions = []map[string]any{
		{"id": "9", "command": "npm start", "created": time.Now().Add(-time.Minute).Format(time.RFC3339)},
		{"id": "4", "command": "npm start", "created": time.Now().Add(-time.Hour).Format(time.RFC3339)},
	}

	e := newEnv(t, newAttach(), "--sprite", "dev", "npm")
	require.NoError(t, runAttach(e.ctx, api.session()))

	assert.Equal(t, "4", api.lastExecQuery().Get("id"), "the oldest matching session is chosen")
	assert.Contains(t, e.errOut.String(), "Attaching to session 4 (npm start)")
}

func TestAttachWithNoSessions(t *testing.T) {
	api := newSpritesAPI(t)
	api.addSprite("dev")

	// Named a session: there is none to find.
	e := newEnv(t, newAttach(), "--sprite", "dev", "3")
	err := runAttach(e.ctx, api.session())
	require.ErrorContains(t, err, "no running sessions in sprite dev")
	assert.Contains(t, flyerr.GetErrorSuggestion(err), "fly sprites console")

	// Named nothing, and nobody to offer a new console to.
	e = newEnv(t, newAttach(), "--sprite", "dev")
	require.ErrorContains(t, runAttach(e.ctx, api.session()), "no running sessions in sprite dev")

	assert.Nil(t, api.lastExecQuery(), "no console is started without being asked")
}

func TestOpenEndedReader(t *testing.T) {
	r := openEndedReader{strings.NewReader("abc")}

	buf := make([]byte, 8)
	n, err := r.Read(buf)
	require.NoError(t, err)
	assert.Equal(t, "abc", string(buf[:n]))

	// At the end of its input it waits rather than reporting the end.
	done := make(chan struct{})
	go func() {
		_, _ = r.Read(buf)
		close(done)
	}()

	select {
	case <-done:
		t.Fatal("the reader reported the end of its input")
	case <-time.After(100 * time.Millisecond):
	}
}

func TestResetTerminal(t *testing.T) {
	var out strings.Builder
	resetTerminal(&out)

	// Bracketed paste off, attributes reset, cursor shown.
	for _, seq := range []string{"\033[?2004l", "\033[0m", "\033[?25h"} {
		assert.Contains(t, out.String(), seq)
	}
}

func TestWatchResizeStops(t *testing.T) {
	// Not a terminal, so there is no size to report; stopping must still be
	// clean and must not leave the watcher running.
	stop := watchResize(-1, func(uint16, uint16) { t.Error("no size should be reported for a non-terminal") })
	stop()
}

func TestRemoteErrorIsNotAnExitStatus(t *testing.T) {
	api := newSpritesAPI(t)
	api.addSprite("dev")

	api.exec = func(c *execConn) {
		c.text(map[string]any{"type": "error", "error": "exec failed: no such file"})
	}

	e := newEnv(t, newExec(), "--sprite", "dev", "--no-stdin", "nope")
	err := runExec(e.ctx, api.session())

	// A command that could not run is an error to print, not a status to
	// relay silently.
	var exitErr *flyerr.ExitCodeError
	require.False(t, errors.As(err, &exitErr))
	require.ErrorContains(t, err, "no such file")
}

// fakeTerminal is a local terminal with scripted keystrokes.
type fakeTerminal struct {
	input      io.Reader
	rows, cols uint16

	raw      atomic.Bool
	restored atomic.Bool
	resize   chan func(rows, cols uint16)
	stopped  atomic.Bool
}

func (f *fakeTerminal) Input() io.Reader { return f.input }

func (f *fakeTerminal) MakeRaw() (func(), error) {
	f.raw.Store(true)
	return func() { f.restored.Store(true) }, nil
}

func (f *fakeTerminal) Size() (uint16, uint16, bool) { return f.rows, f.cols, f.rows > 0 }

func (f *fakeTerminal) WatchResize(fn func(rows, cols uint16)) func() {
	if f.resize != nil {
		f.resize <- fn
	}

	return func() { f.stopped.Store(true) }
}

// atTerminal makes runRemote see tty as the user's terminal.
func atTerminal(t *testing.T, tty *fakeTerminal) {
	t.Helper()

	original := openTerminal
	openTerminal = func() localTerminal { return tty }

	t.Cleanup(func() { openTerminal = original })
}

func TestInteractiveSession(t *testing.T) {
	api := newSpritesAPI(t)
	api.addSprite("dev")

	keys, typing := io.Pipe()
	tty := &fakeTerminal{input: keys, rows: 40, cols: 120, resize: make(chan func(uint16, uint16), 1)}
	atTerminal(t, tty)

	sizes := make(chan map[string]any, 4)
	api.exec = func(c *execConn) {
		c.stdout("prompt$ ")

		var typed []byte
		for msg := range c.messages {
			if msg.control != nil && msg.control["type"] == "resize" {
				sizes <- msg.control
			}

			typed = append(typed, msg.stdin...)
			if strings.HasSuffix(string(typed), "exit\r") {
				break
			}
		}

		c.stdout("bye")
		c.exit(5)
	}

	e := newEnv(t, newExec(), "--sprite", "dev", "--tty", "bash")
	e.ios.SetStdoutTTY(true)

	done := make(chan error, 1)
	go func() { done <- runExec(e.ctx, api.session()) }()

	// The size is sent once the session is running.
	first := <-sizes
	assert.EqualValues(t, 40, first["rows"])
	assert.EqualValues(t, 120, first["cols"])

	// Resizing the window reaches the remote terminal.
	(<-tty.resize)(50, 150)

	second := <-sizes
	assert.EqualValues(t, 50, second["rows"])
	assert.EqualValues(t, 150, second["cols"])

	// Keystrokes pass through as typed.
	_, err := typing.Write([]byte("exit\r"))
	require.NoError(t, err)

	assert.Equal(t, 5, exitCode(t, <-done))

	out := e.out.String()
	assert.True(t, strings.HasPrefix(out, "\033[?1004;2004sprompt$ bye"), "modes are saved before the session")
	assert.Contains(t, out, "\033[?25h", "the terminal is reset after the session")
	assert.Contains(t, out, "\033[<u", "a pushed kitty keyboard mode is popped")
	assert.Contains(t, out, "\033[?1004;2004r", "saved modes are restored")

	assert.True(t, tty.raw.Load(), "the terminal is put in raw mode")
	assert.True(t, tty.restored.Load(), "and taken out of it afterwards")
	assert.True(t, tty.stopped.Load(), "and no longer watched for resizes")

	query := api.lastExecQuery()
	assert.Equal(t, "true", query.Get("tty"))
	assert.Equal(t, "40", query.Get("rows"))
	assert.Equal(t, "120", query.Get("cols"))
}

func TestInteractiveSessionDetach(t *testing.T) {
	api := newSpritesAPI(t)
	api.addSprite("dev").sessions = []map[string]any{{"id": "3", "command": "bash", "tty": true}}

	keys, typing := io.Pipe()
	tty := &fakeTerminal{input: keys, rows: 24, cols: 80}
	atTerminal(t, tty)

	var (
		received  atomic.Value
		signalled atomic.Bool
	)

	release := make(chan struct{})
	t.Cleanup(func() { close(release) })

	api.exec = func(c *execConn) {
		var typed []byte

		for {
			select {
			case msg, ok := <-c.messages:
				if !ok {
					return
				}

				if msg.control != nil && msg.control["type"] == "signal" {
					signalled.Store(true)
				}

				typed = append(typed, msg.stdin...)
				received.Store(string(typed))
			case <-release:
				return
			}
		}
	}

	e := newEnv(t, newSessionsAttach(), "--sprite", "dev", "3")

	done := make(chan error, 1)
	go func() { done <- runSessionsAttach(e.ctx, api.session()) }()

	// Type something, then Ctrl+\.
	_, err := typing.Write([]byte("ls\r"))
	require.NoError(t, err)

	require.Eventually(t, func() bool { return received.Load() == "ls\r" }, 5*time.Second, 10*time.Millisecond)

	_, err = typing.Write([]byte{detachChar})
	require.NoError(t, err)

	select {
	case err := <-done:
		require.NoError(t, err, "detaching is not a failure")
	case <-time.After(10 * time.Second):
		t.Fatal("the session did not detach")
	}

	assert.Contains(t, e.errOut.String(), "Detached from session 3.")
	assert.True(t, tty.restored.Load(), "the terminal is restored on detach")

	// The session is left as it was: not signalled, and not sent the detach
	// key as if it were input.
	time.Sleep(100 * time.Millisecond)
	assert.False(t, signalled.Load(), "detaching must not signal the session")
	assert.Equal(t, "ls\r", received.Load(), "the detach key itself is not sent to the session")
	assert.Equal(t, "3", api.lastExecQuery().Get("id"))
}

func TestCreateOpensConsoleAtTerminal(t *testing.T) {
	api := newSpritesAPI(t)

	keys, typing := io.Pipe()
	defer typing.Close() //nolint:errcheck

	atTerminal(t, &fakeTerminal{input: keys, rows: 24, cols: 80})

	api.exec = func(c *execConn) {
		c.stdout("welcome")
		c.exit(0)
	}

	t.Setenv("TERM", "xterm-256color")

	e := newEnv(t, newCreate(), "my-sprite")
	e.ios.SetStdinTTY(true)
	e.ios.SetStdoutTTY(true)

	require.NoError(t, runCreate(e.ctx, api.session()))

	assert.Contains(t, e.out.String(), "Connecting to console...")
	assert.Contains(t, e.out.String(), "welcome")
	assert.Equal(t, []string{consoleCommand}, api.lastExecQuery()["cmd"])

	// --skip-console leaves it at creating the sprite.
	e = newEnv(t, newCreate(), "--skip-console", "second")
	e.ios.SetStdinTTY(true)
	e.ios.SetStdoutTTY(true)

	require.NoError(t, runCreate(e.ctx, api.session()))
	assert.NotContains(t, e.out.String(), "Connecting to console")
	assert.Len(t, api.execQueries, 1)
}

func TestOpenTerminalWithoutTerminal(t *testing.T) {
	// Under `go test` stdin is not a terminal, so there is none to open.
	if tty := openTerminal(); tty != nil {
		t.Skip("stdin is a terminal")
	}
}
