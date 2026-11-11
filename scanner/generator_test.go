package scanner

import (
	"errors"
	"os"
	"os/exec"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunGeneratorAddsStderrToError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses sh")
	}

	err := runGenerator(exec.Command("sh", "-c", `for i in 1 2 3 4 5 6 7; do echo "line $i" >&2; done; echo >&2; exit 3`), false)

	require.Error(t, err)
	assert.Equal(t, "exit status 3:\nline 3\nline 4\nline 5\nline 6\nline 7", err.Error())

	var exitErr *exec.ExitError
	require.True(t, errors.As(err, &exitErr), "exit code must stay reachable")
	assert.Equal(t, 3, exitErr.ExitCode())

	assert.NoError(t, runGenerator(exec.Command("sh", "-c", "echo fine >&2"), false))
}

// An agent or CI runner often gives flyctl a stdin pipe it never closes. A
// generator asking "overwrite Dockerfile?" used to wait on that pipe forever;
// it should read EOF instead, as it does under `fly launch < /dev/null`.
func TestRunGeneratorDoesNotWaitOnOpenStdinPipe(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses sh")
	}

	r, w, err := os.Pipe()
	require.NoError(t, err)
	defer r.Close()
	defer w.Close() // held open for the whole test, like an agent's stdin

	oldStdin := os.Stdin
	os.Stdin = r
	defer func() { os.Stdin = oldStdin }()

	done := make(chan error, 1)
	go func() {
		// `read` fails on EOF; exit 0 then, so only a hang fails the test.
		done <- runGenerator(exec.Command("sh", "-c", `read answer; exit 0`), false)
	}()

	select {
	case err := <-done:
		assert.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("generator is waiting on flyctl's open stdin pipe")
	}
}

// redirectStdin points os.Stdin at a file holding contents for the test.
func redirectStdin(t *testing.T, contents string) {
	t.Helper()

	f, err := os.CreateTemp(t.TempDir(), "stdin")
	require.NoError(t, err)
	t.Cleanup(func() { f.Close() })
	_, err = f.WriteString(contents)
	require.NoError(t, err)
	_, err = f.Seek(0, 0)
	require.NoError(t, err)

	oldStdin := os.Stdin
	os.Stdin = f
	t.Cleanup(func() { os.Stdin = oldStdin })
}

// `fly launch < answers.txt` used to feed the file to the generator. When no
// one can answer, the generator now sees an empty stdin instead.
func TestRunGeneratorNonInteractiveGetsEmptyStdin(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses sh")
	}
	redirectStdin(t, "y\n")

	// Fails if the generator read anything at all.
	err := runGenerator(exec.Command("sh", "-c", `read answer; [ -z "$answer" ]`), false)
	assert.NoError(t, err, "the generator should get an empty stdin, not flyctl's")
}

func TestRunGeneratorInteractiveGetsFlyctlsStdin(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses sh")
	}
	redirectStdin(t, "y\n")

	err := runGenerator(exec.Command("sh", "-c", `read answer; [ "$answer" = y ]`), true)
	assert.NoError(t, err, "someone at the terminal can answer the generator's questions")
}

// exec.Cmd treats a non-nil io.Reader holding a nil *os.File differently from
// a nil io.Reader, so generatorStdin must return a true nil.
func TestGeneratorStdin(t *testing.T) {
	assert.True(t, generatorStdin(false) == nil)
	assert.Equal(t, os.Stdin, generatorStdin(true))
}

func TestWithSkip(t *testing.T) {
	assert.Equal(t, []string{"--skip"}, withSkip(nil))
	assert.Equal(t, []string{"--postgres", "--skip"}, withSkip([]string{"--postgres"}))
	assert.Equal(t, []string{"--skip", "--postgres"}, withSkip([]string{"--skip", "--postgres"}), "--skip isn't added twice")
}

func TestTailWriterKeepsLastBytes(t *testing.T) {
	w := &tailWriter{max: 4}
	_, _ = w.Write([]byte("abc"))
	_, _ = w.Write([]byte("defg"))

	assert.Equal(t, "defg", string(w.buf))
}
