package scanner

import (
	"errors"
	"os/exec"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunGeneratorAddsStderrToError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses sh")
	}

	err := runGenerator(exec.Command("sh", "-c", `for i in 1 2 3 4 5 6 7; do echo "line $i" >&2; done; echo >&2; exit 3`))

	require.Error(t, err)
	assert.Equal(t, "exit status 3:\nline 3\nline 4\nline 5\nline 6\nline 7", err.Error())

	var exitErr *exec.ExitError
	require.True(t, errors.As(err, &exitErr), "exit code must stay reachable")
	assert.Equal(t, 3, exitErr.ExitCode())

	assert.NoError(t, runGenerator(exec.Command("sh", "-c", "echo fine >&2")))
}

func TestTailWriterKeepsLastBytes(t *testing.T) {
	w := &tailWriter{max: 4}
	_, _ = w.Write([]byte("abc"))
	_, _ = w.Write([]byte("defg"))

	assert.Equal(t, "defg", string(w.buf))
}
