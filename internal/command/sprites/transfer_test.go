package sprites

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"

	"github.com/superfly/flyctl/internal/flyerr"
)

// localTree writes files, given as path to contents, under a new directory.
func localTree(t *testing.T, files map[string]string) string {
	t.Helper()

	root := t.TempDir()

	for name, data := range files {
		full := filepath.Join(root, filepath.FromSlash(name))
		require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
		require.NoError(t, os.WriteFile(full, []byte(data), 0o644))
	}

	return root
}

func remoteFiles(sp *fakeSprite) map[string]string {
	files := map[string]string{}
	for name, file := range sp.files {
		files[name] = string(file.data)
	}

	return files
}

func TestFilePushSingleFile(t *testing.T) {
	api := newSpritesAPI(t)
	sp := api.addSprite("dev")
	src := localTree(t, map[string]string{"app.conf": "port=1"})

	e := newEnv(t, newFilePush(), "--sprite", "dev", filepath.Join(src, "app.conf"), "/tmp/app.conf")
	require.NoError(t, runFilePush(e.ctx, api.session()))

	assert.Equal(t, map[string]string{"/tmp/app.conf": "port=1"}, remoteFiles(sp))
	assert.Equal(t, "/tmp/app.conf\n", e.out.String())
}

func TestFilePushIntoDirectory(t *testing.T) {
	api := newSpritesAPI(t)
	sp := api.addSprite("dev")
	src := localTree(t, map[string]string{"a.txt": "A", "b.txt": "B"})

	// An existing directory takes the file under its own name, with or
	// without a trailing slash.
	e := newEnv(t, newFilePush(), "--sprite", "dev", filepath.Join(src, "a.txt"), "/tmp")
	require.NoError(t, runFilePush(e.ctx, api.session()))

	// Several sources always go into the destination directory.
	e = newEnv(t, newFilePush(), "--sprite", "dev", filepath.Join(src, "a.txt"), filepath.Join(src, "b.txt"), "/home/sprite/")
	require.NoError(t, runFilePush(e.ctx, api.session()))

	assert.Equal(t, map[string]string{
		"/tmp/a.txt":         "A",
		"/home/sprite/a.txt": "A",
		"/home/sprite/b.txt": "B",
	}, remoteFiles(sp))
}

func TestFilePushRelativeAndNamedSprite(t *testing.T) {
	api := newSpritesAPI(t)
	api.addSprite("dev")
	other := api.addSprite("other")
	src := localTree(t, map[string]string{"a.txt": "A"})

	// "<sprite>:<path>" names the sprite inline, and a relative path is
	// resolved against the home directory.
	e := newEnv(t, newFilePush(), "--sprite", "dev", filepath.Join(src, "a.txt"), "other:notes.txt")
	require.NoError(t, runFilePush(e.ctx, api.session()))

	assert.Equal(t, map[string]string{remoteHome + "/notes.txt": "A"}, remoteFiles(other))
}

func TestFilePushMissingParent(t *testing.T) {
	api := newSpritesAPI(t)
	sp := api.addSprite("dev")
	src := localTree(t, map[string]string{"a.txt": "A"})
	file := filepath.Join(src, "a.txt")

	for _, dest := range []string{"/tmp/nope/a.txt", "/tmp/nope/"} {
		e := newEnv(t, newFilePush(), "--sprite", "dev", file, dest)
		err := runFilePush(e.ctx, api.session())
		require.ErrorContains(t, err, "remote directory /tmp/nope does not exist", dest)
		assert.Contains(t, flyerr.GetErrorSuggestion(err), "--parents")
	}

	assert.Empty(t, sp.files, "nothing is created without --parents")

	e := newEnv(t, newFilePush(), "--sprite", "dev", "--parents", file, "/tmp/nope/deep/a.txt")
	require.NoError(t, runFilePush(e.ctx, api.session()))
	assert.Contains(t, sp.files, "/tmp/nope/deep/a.txt")
}

func TestFilePushReportsRealErrorNotMissingDirectory(t *testing.T) {
	api := newSpritesAPI(t)
	api.addSprite("dev")
	src := localTree(t, map[string]string{"a.txt": "A"})

	e := newEnv(t, newFilePush(), "--sprite", "dev", filepath.Join(src, "a.txt"), "/tmp/a.txt")
	err := runFilePush(e.ctx, sessionWithToken(api, "wrong"))

	// A rejected token is not a missing directory, and must be recognisable
	// as a rejected token so the saved one gets replaced.
	require.ErrorContains(t, err, "failed to check remote directory /tmp")
	assert.NotContains(t, err.Error(), "does not exist")
	assert.Empty(t, flyerr.GetErrorSuggestion(err))
}

func TestFilePushDirectory(t *testing.T) {
	api := newSpritesAPI(t)
	sp := api.addSprite("dev")

	src := localTree(t, map[string]string{"a.txt": "A", "sub/b.txt": "B"})
	require.NoError(t, os.MkdirAll(filepath.Join(src, "empty", "nested"), 0o755))

	// Without --recursive a directory is refused.
	e := newEnv(t, newFilePush(), "--sprite", "dev", src, "/tmp/tree")
	err := runFilePush(e.ctx, api.session())
	require.ErrorContains(t, err, "is a directory")
	assert.Contains(t, flyerr.GetErrorSuggestion(err), "--recursive")

	e = newEnv(t, newFilePush(), "--sprite", "dev", "--recursive", src, "/tmp/tree")
	require.NoError(t, runFilePush(e.ctx, api.session()))

	assert.Equal(t, map[string]string{"/tmp/tree/a.txt": "A", "/tmp/tree/sub/b.txt": "B"}, remoteFiles(sp))

	// Directories with no files in them are made with mkdir in the sprite,
	// and no placeholder files are left behind.
	query := api.lastExecQuery()
	require.NotNil(t, query)
	assert.Equal(t, []string{"mkdir", "-p", "--", "/tmp/tree/empty/nested"}, query["cmd"])
}

func TestFilePushSkipsSpecialFiles(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges on Windows")
	}

	api := newSpritesAPI(t)
	sp := api.addSprite("dev")

	src := localTree(t, map[string]string{"real.txt": "R"})
	require.NoError(t, os.Symlink("real.txt", filepath.Join(src, "link.txt")))

	e := newEnv(t, newFilePush(), "--sprite", "dev", "-r", src, "/tmp/tree")
	require.NoError(t, runFilePush(e.ctx, api.session()))

	assert.Equal(t, map[string]string{"/tmp/tree/real.txt": "R"}, remoteFiles(sp))
	assert.Contains(t, e.out.String(), "link.txt: not a regular file")
}

func TestFilePushMissingLocalFile(t *testing.T) {
	api := newSpritesAPI(t)
	api.addSprite("dev")

	e := newEnv(t, newFilePush(), "--sprite", "dev", "/does/not/exist", "/tmp/x")
	require.Error(t, runFilePush(e.ctx, api.session()))
}

func TestFilePullSingleFile(t *testing.T) {
	api := newSpritesAPI(t)
	api.addSprite("dev").putFile("/var/log/app.log", "line\n", "640")

	dest := t.TempDir()

	// To an existing directory: keeps its name.
	e := newEnv(t, newFilePull(), "--sprite", "dev", "/var/log/app.log", dest)
	require.NoError(t, runFilePull(e.ctx, api.session()))

	data, err := os.ReadFile(filepath.Join(dest, "app.log"))
	require.NoError(t, err)
	assert.Equal(t, "line\n", string(data))

	if runtime.GOOS != "windows" {
		info, err := os.Stat(filepath.Join(dest, "app.log"))
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o640), info.Mode().Perm(), "the remote permissions are kept")
	}

	// To a new path: takes that name.
	e = newEnv(t, newFilePull(), "--sprite", "dev", "/var/log/app.log", filepath.Join(dest, "renamed.log"))
	require.NoError(t, runFilePull(e.ctx, api.session()))
	assert.FileExists(t, filepath.Join(dest, "renamed.log"))
}

func TestFilePullMissingLocalParent(t *testing.T) {
	api := newSpritesAPI(t)
	api.addSprite("dev").putFile("/tmp/a.txt", "A", "644")

	dest := filepath.Join(t.TempDir(), "new", "deep", "a.txt")

	e := newEnv(t, newFilePull(), "--sprite", "dev", "/tmp/a.txt", dest)
	err := runFilePull(e.ctx, api.session())
	require.ErrorContains(t, err, "does not exist")
	assert.Contains(t, flyerr.GetErrorSuggestion(err), "--parents")

	e = newEnv(t, newFilePull(), "--sprite", "dev", "-p", "/tmp/a.txt", dest)
	require.NoError(t, runFilePull(e.ctx, api.session()))
	assert.FileExists(t, dest)
}

func TestFilePullSeveralSourcesFromDifferentSprites(t *testing.T) {
	api := newSpritesAPI(t)
	api.addSprite("dev").putFile("/tmp/a.txt", "A", "644")
	api.addSprite("other").putFile("/tmp/b.txt", "B", "644")

	// A directory that does not exist yet is made to hold them.
	dest := filepath.Join(t.TempDir(), "downloads")

	e := newEnv(t, newFilePull(), "dev:/tmp/a.txt", "other:/tmp/b.txt", dest)
	require.NoError(t, runFilePull(e.ctx, api.session()))

	assert.FileExists(t, filepath.Join(dest, "a.txt"))
	assert.FileExists(t, filepath.Join(dest, "b.txt"))

	// Several sources cannot go to one existing file.
	e = newEnv(t, newFilePull(), "dev:/tmp/a.txt", "other:/tmp/b.txt", filepath.Join(dest, "a.txt"))
	require.ErrorContains(t, runFilePull(e.ctx, api.session()), "must be a directory")
}

func TestFilePullDirectory(t *testing.T) {
	api := newSpritesAPI(t)
	sp := api.addSprite("dev")
	sp.putFile("/srv/app/a.txt", "A", "644")
	sp.putFile("/srv/app/sub/b.txt", "B", "644")
	sp.dirs["/srv/app/empty"] = true

	root := t.TempDir()

	e := newEnv(t, newFilePull(), "--sprite", "dev", "/srv/app", filepath.Join(root, "new"))
	err := runFilePull(e.ctx, api.session())
	require.ErrorContains(t, err, "is a directory in the sprite")

	// To a new path, the directory is copied as that path. This is the
	// example in the command's help, trailing slash included.
	fresh := filepath.Join(root, "dist") + string(filepath.Separator)

	e = newEnv(t, newFilePull(), "--sprite", "dev", "-r", "/srv/app/", fresh)
	require.NoError(t, runFilePull(e.ctx, api.session()))

	for _, name := range []string{"a.txt", filepath.Join("sub", "b.txt")} {
		assert.FileExists(t, filepath.Join(root, "dist", name))
	}
	assert.DirExists(t, filepath.Join(root, "dist", "empty"))

	// To a directory that already exists, it goes inside under its own name
	// rather than being merged over what is there.
	existing := filepath.Join(root, "existing")
	require.NoError(t, os.Mkdir(existing, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(existing, "a.txt"), []byte("mine"), 0o644))

	e = newEnv(t, newFilePull(), "--sprite", "dev", "-r", "/srv/app", existing)
	require.NoError(t, runFilePull(e.ctx, api.session()))

	mine, err := os.ReadFile(filepath.Join(existing, "a.txt"))
	require.NoError(t, err)
	assert.Equal(t, "mine", string(mine), "a local file must not be overwritten by the merge")
	assert.FileExists(t, filepath.Join(existing, "app", "a.txt"))
}

func TestFilePullSkipsUnreadableEntries(t *testing.T) {
	api := newSpritesAPI(t)
	sp := api.addSprite("dev")
	sp.putFile("/srv/app/a.txt", "A", "644")
	sp.putFile("/srv/app/z.txt", "Z", "644")

	// The API lists symlinks as files; these two fail when read.
	sp.unreadable["/srv/app/dangling"] = true
	sp.unreadable["/srv/app/loop"] = true

	dest := filepath.Join(t.TempDir(), "out")

	e := newEnv(t, newFilePull(), "--sprite", "dev", "-r", "/srv/app", dest)
	err := runFilePull(e.ctx, api.session())

	// Everything readable is copied, including what sorts after the bad
	// entries, and the command still reports that it was not a full copy.
	require.ErrorContains(t, err, "2 entries could not be copied")
	assert.FileExists(t, filepath.Join(dest, "a.txt"))
	assert.FileExists(t, filepath.Join(dest, "z.txt"))
	assert.Contains(t, e.out.String(), "skipping /srv/app/dangling")
	assert.Contains(t, e.out.String(), "skipping /srv/app/loop")
}

func TestFilePullParentPathStaysInsideDestination(t *testing.T) {
	api := newSpritesAPI(t)
	api.addSprite("dev").putFile("/home/victim.txt", "remote replacement", "0644")
	root := t.TempDir()
	target := filepath.Join(root, "backup")
	require.NoError(t, os.Mkdir(target, 0o755))
	victim := filepath.Join(root, "victim.txt")
	require.NoError(t, os.WriteFile(victim, []byte("local contents"), 0o644))

	e := newEnv(t, newFilePull(), "--sprite", "dev", "-r", "..", target)
	require.NoError(t, runFilePull(e.ctx, api.session()))
	data, err := os.ReadFile(victim)
	require.NoError(t, err)
	assert.Equal(t, "local contents", string(data))
	data, err = os.ReadFile(filepath.Join(target, "home", "victim.txt"))
	require.NoError(t, err)
	assert.Equal(t, "remote replacement", string(data))
}

func TestFilePushRejectsEscapingSourceName(t *testing.T) {
	root := localTree(t, map[string]string{"a.txt": "A", "sub/b.txt": "B"})
	api := newSpritesAPI(t)
	sp := api.addSprite("dev")
	sp.dirs["/tmp/uploads"] = true

	e := newEnv(t, newFilePush(), "--sprite", "dev", "-r", "..", "b.txt", "/tmp/uploads")
	t.Chdir(filepath.Join(root, "sub"))
	require.ErrorContains(t, runFilePush(e.ctx, api.session()), "specify its resolved path")
	assert.Empty(t, sp.files, "no files may be uploaded outside the requested destination")
}

func TestFilePullStopsWhenInterrupted(t *testing.T) {
	api := newSpritesAPI(t)
	sp := api.addSprite("dev")

	for i := range 20 {
		sp.putFile(fmt.Sprintf("/srv/app/%02d.txt", i), "x", "644")
	}

	dest := filepath.Join(t.TempDir(), "out")

	e := newEnv(t, newFilePull(), "--sprite", "dev", "-r", "/srv/app", dest)

	ctx, cancel := context.WithCancel(e.ctx)
	cancel()

	require.ErrorIs(t, runFilePull(ctx, api.session()), context.Canceled)

	entries, _ := os.ReadDir(dest)
	assert.Empty(t, entries, "nothing more is downloaded after an interrupt")
}

func TestFilePullMissingRemote(t *testing.T) {
	api := newSpritesAPI(t)
	api.addSprite("dev")

	e := newEnv(t, newFilePull(), "--sprite", "dev", "/nope", t.TempDir())
	require.ErrorContains(t, runFilePull(e.ctx, api.session()), "/nope")
}

func TestFileEditFromStdin(t *testing.T) {
	api := newSpritesAPI(t)
	sp := api.addSprite("dev")
	sp.putFile("/etc/app.conf", "old", "600")

	e := newEnv(t, newFileEdit(), "--sprite", "dev", "/etc/app.conf")
	e.in.WriteString("new contents")

	require.NoError(t, runFileEdit(e.ctx, api.session()))

	assert.Equal(t, "new contents", string(sp.files["/etc/app.conf"].data))
	assert.Equal(t, "600", sp.files["/etc/app.conf"].mode, "the file keeps its permissions")
	assert.Contains(t, e.errOut.String(), "Updated /etc/app.conf")
}

func TestFileEditUnchanged(t *testing.T) {
	api := newSpritesAPI(t)
	api.addSprite("dev").putFile("/etc/app.conf", "same", "644")

	e := newEnv(t, newFileEdit(), "--sprite", "dev", "/etc/app.conf")
	e.in.WriteString("same")

	require.NoError(t, runFileEdit(e.ctx, api.session()))
	assert.Contains(t, e.errOut.String(), "No changes made")

	for _, req := range api.requested() {
		assert.NotContains(t, req, "PUT", "an unchanged file is not uploaded")
	}
}

func TestFileEditRejectsDirectoriesAndMissingFiles(t *testing.T) {
	api := newSpritesAPI(t)
	api.addSprite("dev")

	e := newEnv(t, newFileEdit(), "--sprite", "dev", "/tmp")
	require.ErrorContains(t, runFileEdit(e.ctx, api.session()), "/tmp is a directory")

	e = newEnv(t, newFileEdit(), "--sprite", "dev", "/tmp/nope")
	require.ErrorContains(t, runFileEdit(e.ctx, api.session()), "/tmp/nope")
}

func TestFileEditWithEditor(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a shell script as the editor")
	}

	api := newSpritesAPI(t)
	sp := api.addSprite("dev")
	sp.putFile("/etc/app.conf", "port=1\n", "644")

	// An "editor" that appends a line to the file it is given. $VISUAL wins
	// over $EDITOR, and may carry arguments.
	editor := filepath.Join(t.TempDir(), "editor.sh")
	require.NoError(t, os.WriteFile(editor, []byte("#!/bin/sh\necho \"$1\" >> \"$2\"\n"), 0o755))

	t.Setenv("VISUAL", editor+" debug=true")
	t.Setenv("EDITOR", "false")

	e := newEnv(t, newFileEdit(), "--sprite", "dev", "/etc/app.conf")
	e.ios.SetStdinTTY(true)

	require.NoError(t, runFileEdit(e.ctx, api.session()))
	assert.Equal(t, "port=1\ndebug=true\n", string(sp.files["/etc/app.conf"].data))
}

func TestFileEditEditorFailureUploadsNothing(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses false as the editor")
	}

	api := newSpritesAPI(t)
	sp := api.addSprite("dev")
	sp.putFile("/etc/app.conf", "port=1\n", "644")

	t.Setenv("VISUAL", "")
	t.Setenv("EDITOR", "false")

	e := newEnv(t, newFileEdit(), "--sprite", "dev", "/etc/app.conf")
	e.ios.SetStdinTTY(true)

	require.ErrorContains(t, runFileEdit(e.ctx, api.session()), "nothing was uploaded")
	assert.Equal(t, "port=1\n", string(sp.files["/etc/app.conf"].data))
}

func TestFindEditor(t *testing.T) {
	t.Setenv("VISUAL", "")
	t.Setenv("EDITOR", `code --wait "my file"`)

	editor, err := findEditor()
	require.NoError(t, err)
	assert.Equal(t, []string{"code", "--wait", "my file"}, editor)

	t.Setenv("EDITOR", `"unterminated`)
	_, err = findEditor()
	require.ErrorContains(t, err, "could not parse $EDITOR")

	// With neither set, a common editor is found on PATH, or there is advice.
	t.Setenv("EDITOR", "")
	t.Setenv("PATH", t.TempDir())

	_, err = findEditor()
	require.ErrorContains(t, err, "no editor found")
	assert.Contains(t, flyerr.GetErrorSuggestion(err), "$EDITOR")
}

func freePort(t *testing.T) int {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer listener.Close() //nolint:errcheck

	return listener.Addr().(*net.TCPAddr).Port
}

func TestProxyPorts(t *testing.T) {
	api := newSpritesAPI(t)
	api.addSprite("dev")

	api.tcp = func(port int, conn io.ReadWriter) {
		fmt.Fprintf(conn, "remote port %d", port)
	}

	local := freePort(t)

	e := newEnv(t, newProxy(), "--sprite", "dev", fmt.Sprintf("%d:5432", local))

	ctx, cancel := context.WithCancel(e.ctx)
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- runProxy(ctx, api.session()) }()

	var (
		conn net.Conn
		err  error
	)

	require.Eventually(t, func() bool {
		conn, err = net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", local))
		return err == nil
	}, 10*time.Second, 20*time.Millisecond)

	reply, err := io.ReadAll(conn)
	require.NoError(t, err)
	assert.Equal(t, "remote port 5432", string(reply))

	// Ctrl+C ends the proxy cleanly.
	cancel()

	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("proxy did not stop when cancelled")
	}

	assert.Contains(t, e.out.String(), fmt.Sprintf("Forwarding 127.0.0.1:%d -> dev port 5432", local))
}

func TestProxyPortInUse(t *testing.T) {
	api := newSpritesAPI(t)
	api.addSprite("dev")

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer listener.Close() //nolint:errcheck

	port := listener.Addr().(*net.TCPAddr).Port

	e := newEnv(t, newProxy(), "--sprite", "dev", fmt.Sprint(port))
	err = runProxy(e.ctx, api.session())

	require.ErrorContains(t, err, "failed to start proxy")
	assert.Contains(t, flyerr.GetErrorSuggestion(err), "already in use")
}

func TestProxyStdio(t *testing.T) {
	api := newSpritesAPI(t)
	api.addSprite("dev")

	var gotPort int
	api.tcp = func(port int, conn io.ReadWriter) {
		gotPort = port
		fmt.Fprint(conn, "SSH-2.0-banner")
	}

	// A bare port means that port on the sprite itself.
	e := newEnv(t, newProxy(), "--sprite", "dev", "--stdio", "22")
	require.NoError(t, runProxy(e.ctx, api.session()))

	assert.Equal(t, "SSH-2.0-banner", e.out.String())
	assert.Equal(t, 22, gotPort)
}

func TestProxyStdioConnectFailure(t *testing.T) {
	api := newSpritesAPI(t)
	api.addSprite("dev")

	e := newEnv(t, newProxy(), "--sprite", "dev", "-W", ":22")
	require.ErrorContains(t, runProxy(e.ctx, sessionWithToken(api, "wrong")), "failed to connect to :22 in sprite dev")
}

// sshClient connects an SSH client to serveSSH over a local socket.
func sshClient(t *testing.T, api *spritesAPI) *ssh.Client {
	t.Helper()

	// A real socket pair: both ends of an SSH handshake write first, which an
	// unbuffered in-memory pipe cannot take.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer listener.Close() //nolint:errcheck

	clientSide, err := net.Dial("tcp", listener.Addr().String())
	require.NoError(t, err)

	serverSide, err := listener.Accept()
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	s := api.session()

	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = serveSSH(ctx, serverSide, s, s.Client.Sprite("dev"))
	}()

	hostKey, err := sshHostKey(s, "dev")
	require.NoError(t, err)

	conn, channels, requests, err := ssh.NewClientConn(clientSide, "dev", &ssh.ClientConfig{
		User: "sprite",
		// The derived host key is what a client's known_hosts would hold.
		HostKeyCallback: ssh.FixedHostKey(hostKey.PublicKey()),
		Timeout:         5 * time.Second,
	})
	require.NoError(t, err)

	client := ssh.NewClient(conn, channels, requests)

	t.Cleanup(func() {
		_ = client.Close()
		cancel()

		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("the SSH server did not stop")
		}
	})

	return client
}

func TestProxySSHExec(t *testing.T) {
	api := newSpritesAPI(t)
	api.addSprite("dev")

	api.exec = func(c *execConn) {
		c.stdout("got:" + c.readStdin())
		c.stderr("warning\n")
		c.exit(3)
	}

	session, err := sshClient(t, api).NewSession()
	require.NoError(t, err)

	var stdout, stderr bytes.Buffer
	session.Stdin = strings.NewReader("input")
	session.Stdout = &stdout
	session.Stderr = &stderr

	require.NoError(t, session.Setenv("LANG", "C"))

	err = session.Run("echo hi")

	// The remote exit status reaches the SSH client.
	var exitErr *ssh.ExitError
	require.ErrorAs(t, err, &exitErr)
	assert.Equal(t, 3, exitErr.ExitStatus())

	assert.Equal(t, "got:input", stdout.String())
	assert.Equal(t, "warning\n", stderr.String())

	// The command runs as the sprite user, from home, through a login
	// shell, with the client's environment kept.
	query := api.lastExecQuery()
	cmd := query["cmd"]
	assert.Equal(t, "/usr/bin/sudo", cmd[0])
	assert.Contains(t, cmd, "--user=sprite")
	assert.Contains(t, cmd, "--preserve-env=LANG")
	assert.Equal(t, "echo hi", cmd[len(cmd)-1])
	assert.Equal(t, []string{"LANG=C"}, query["env"])
	assert.Empty(t, query.Get("tty"))
}

func TestProxySSHShellWithTerminal(t *testing.T) {
	api := newSpritesAPI(t)
	api.addSprite("dev")

	resized := make(chan map[string]any, 4)
	api.exec = func(c *execConn) {
		c.stdout("prompt$ ")

		// The size is sent once the command is running, then again when the
		// client's window changes.
		for range 2 {
			resized <- c.waitControl("resize")
		}

		c.exit(0)
	}

	session, err := sshClient(t, api).NewSession()
	require.NoError(t, err)

	var stdout bytes.Buffer
	session.Stdout = &stdout

	require.NoError(t, session.RequestPty("xterm-256color", 40, 120, ssh.TerminalModes{}))
	require.NoError(t, session.Shell())

	first := <-resized
	assert.EqualValues(t, 40, first["rows"])
	assert.EqualValues(t, 120, first["cols"])

	require.NoError(t, session.WindowChange(50, 150))

	second := <-resized
	assert.EqualValues(t, 50, second["rows"])
	assert.EqualValues(t, 150, second["cols"])

	require.NoError(t, session.Wait())
	assert.Equal(t, "prompt$ ", stdout.String())

	// With no command, the session is the console wrapper on a terminal.
	query := api.lastExecQuery()
	assert.Equal(t, "true", query.Get("tty"))
	assert.Equal(t, consoleCommand, query["cmd"][len(query["cmd"])-1])
	assert.Contains(t, query["env"], "TERM=xterm-256color")
}

func TestProxySSHSignal(t *testing.T) {
	api := newSpritesAPI(t)
	api.addSprite("dev")

	got := make(chan map[string]any, 1)
	api.exec = func(c *execConn) {
		got <- c.waitControl("signal")
		c.exit(143)
	}

	session, err := sshClient(t, api).NewSession()
	require.NoError(t, err)

	require.NoError(t, session.Start("sleep 600"))
	require.NoError(t, session.Signal(ssh.SIGTERM))

	assert.Equal(t, "TERM", (<-got)["signal"])

	var exitErr *ssh.ExitError
	require.ErrorAs(t, session.Wait(), &exitErr)
	assert.Equal(t, 143, exitErr.ExitStatus())
}

func TestProxySSHRejectsUnsupportedRequests(t *testing.T) {
	api := newSpritesAPI(t)
	api.addSprite("dev")

	client := sshClient(t, api)

	// No sftp subsystem: scp needs -O.
	session, err := client.NewSession()
	require.NoError(t, err)
	require.Error(t, session.RequestSubsystem("sftp"))

	// A session runs one command.
	session, err = client.NewSession()
	require.NoError(t, err)
	require.NoError(t, session.Start("first"))
	require.Error(t, session.Start("second"))

	// Channel types other than sessions and forwards are refused.
	_, _, err = client.OpenChannel("x11", nil)
	require.Error(t, err)
}

func TestProxySSHStartFailure(t *testing.T) {
	fastRetries(t)

	api := newSpritesAPI(t)
	api.addSprite("dev")
	api.failExec, api.failExecStatus = 1, 403

	session, err := sshClient(t, api).NewSession()
	require.NoError(t, err)

	// The command could not start, so the request is refused.
	require.Error(t, session.Run("echo hi"))
}

func TestProxySSHPortForward(t *testing.T) {
	api := newSpritesAPI(t)
	api.addSprite("dev")

	api.tcp = func(port int, conn io.ReadWriter) {
		buf := make([]byte, 4)
		_, _ = io.ReadFull(conn, buf)
		fmt.Fprintf(conn, "%s on %d", buf, port)
	}

	// What `ssh -L` and `ssh -W` open.
	conn, err := sshClient(t, api).Dial("tcp", "localhost:5432")
	require.NoError(t, err)

	_, err = conn.Write([]byte("ping"))
	require.NoError(t, err)

	reply, err := io.ReadAll(conn)
	require.NoError(t, err)
	assert.Equal(t, "ping on 5432", string(reply))
}

func TestStdioConn(t *testing.T) {
	// The connection OpenSSH's ProxyCommand talks over is the process's own
	// stdin and stdout, which have no addresses or deadlines.
	conn := stdioConn{}

	assert.Equal(t, "stdio", conn.LocalAddr().Network())
	assert.Equal(t, "stdio", conn.RemoteAddr().String())
	require.NoError(t, conn.SetDeadline(time.Now()))
	require.NoError(t, conn.SetReadDeadline(time.Now()))
	require.NoError(t, conn.SetWriteDeadline(time.Now()))
}

func TestAPICommand(t *testing.T) {
	if _, err := exec.LookPath("curl"); err != nil {
		t.Skip("curl is not installed")
	}

	api := newSpritesAPI(t)
	api.addSprite("dev").checkpoints = []map[string]any{{"id": "v1"}}

	// A path without /v1 is the target sprite's.
	e := newEnv(t, newAPI(), "--sprite", "dev", "/checkpoints", "-s")
	require.NoError(t, runAPI(e.ctx, api.session()))

	assert.JSONEq(t, `[{"id":"v1"}]`, e.out.String())
	assert.Contains(t, e.errOut.String(), "URL: "+api.URL+"/v1/sprites/dev/checkpoints")

	// An absolute path is used as given, the old "--" separator still works,
	// and curl's own failure status is relayed.
	e = newEnv(t, newAPI(), "--sprite", "dev", "/v1/sprites/ghost", "--", "-s", "--fail")
	err := runAPI(e.ctx, api.session())
	assert.Equal(t, 22, exitCode(t, err))

	requests := api.requested()
	assert.Equal(t, "GET /v1/sprites/ghost", requests[len(requests)-1])

	// The token reaches curl in a file, which is gone afterwards.
	leftovers, _ := filepath.Glob(filepath.Join(os.TempDir(), "fly-sprites-api-*"))
	assert.Empty(t, leftovers)
}

func TestAPICommandWithoutCurl(t *testing.T) {
	api := newSpritesAPI(t)

	t.Setenv("PATH", t.TempDir())

	e := newEnv(t, newAPI(), "/v1/sprites")
	err := runAPI(e.ctx, api.session())
	require.ErrorContains(t, err, "curl is not installed")
	assert.Empty(t, api.requested())
}

// pipeConn joins a read end and a write end into one connection.
type pipeConn struct {
	stdioConn
	r io.ReadCloser
	w io.WriteCloser
}

func (p pipeConn) Read(b []byte) (int, error)  { return p.r.Read(b) }
func (p pipeConn) Write(b []byte) (int, error) { return p.w.Write(b) }
func (p pipeConn) Close() error                { _ = p.r.Close(); return p.w.Close() }

// The way OpenSSH uses it: `fly sprites proxy --ssh` as a ProxyCommand, with
// the SSH connection on the process's own stdin and stdout.
func TestProxySSHOverStdio(t *testing.T) {
	api := newSpritesAPI(t)
	api.addSprite("dev")

	api.exec = func(c *execConn) {
		c.stdout("over stdio")
		c.exit(0)
	}

	toServer, clientWrites, err := os.Pipe()
	require.NoError(t, err)

	clientReads, fromServer, err := os.Pipe()
	require.NoError(t, err)

	realStdin, realStdout := os.Stdin, os.Stdout
	os.Stdin, os.Stdout = toServer, fromServer

	t.Cleanup(func() { os.Stdin, os.Stdout = realStdin, realStdout })

	e := newEnv(t, newProxy(), "--sprite", "dev", "--ssh")
	s := api.session()

	done := make(chan error, 1)
	go func() { done <- runProxy(e.ctx, s) }()

	hostKey, err := sshHostKey(s, "dev")
	require.NoError(t, err)

	conn, channels, requests, err := ssh.NewClientConn(pipeConn{r: clientReads, w: clientWrites}, "dev", &ssh.ClientConfig{
		User:            "sprite",
		HostKeyCallback: ssh.FixedHostKey(hostKey.PublicKey()),
	})
	require.NoError(t, err)

	client := ssh.NewClient(conn, channels, requests)

	session, err := client.NewSession()
	require.NoError(t, err)

	output, err := session.Output("true")
	require.NoError(t, err)
	assert.Equal(t, "over stdio", string(output))

	// When ssh goes away, so does the proxy.
	_ = client.Close()

	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(10 * time.Second):
		t.Fatal("the proxy did not stop when the SSH client disconnected")
	}
}
