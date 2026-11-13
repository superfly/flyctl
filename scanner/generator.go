package scanner

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"
)

// How much of a failed generator's stderr goes into its error.
const (
	generatorErrLines = 5
	generatorErrBytes = 4096
)

// runGenerator runs a Dockerfile generator, or the package manager installing
// one, attached to the terminal. It gets flyctl's stdin only when interactive
// (see generatorStdin). Its stderr still reaches the user as it is
// written; on failure the last lines of it are added to the error too, so
// launch telemetry records why it failed instead of just "exit status 1".
// The *exec.ExitError stays wrapped for errors.As.
func runGenerator(cmd *exec.Cmd, interactive bool) error {
	tail := &tailWriter{max: generatorErrBytes}
	cmd.Stdin = generatorStdin(interactive)
	cmd.Stdout = os.Stdout
	cmd.Stderr = io.MultiWriter(os.Stderr, tail)
	// stderr is a pipe, so don't hang if a grandchild keeps it open after
	// the generator itself has exited successfully.
	cmd.WaitDelay = 5 * time.Second

	err := cmd.Run()
	if err == nil || errors.Is(err, exec.ErrWaitDelay) {
		return nil
	}

	if out := lastLines(string(tail.buf), generatorErrLines); out != "" {
		return fmt.Errorf("%w:\n%s", err, out)
	}

	return err
}

// generatorStdin returns the stdin a generator subprocess gets: flyctl's own
// when someone can answer its prompts, and nil (an empty stdin) otherwise. A
// generator that asks whether to overwrite a file would otherwise wait
// forever on a pipe that an agent or CI runner never closes. On an empty
// stdin it takes its default answer, exactly as under `fly launch < /dev/null`.
func generatorStdin(interactive bool) io.Reader {
	if interactive {
		return os.Stdin
	}

	return nil
}

// tailWriter keeps the last max bytes written to it.
type tailWriter struct {
	max int
	buf []byte
}

func (w *tailWriter) Write(p []byte) (int, error) {
	w.buf = append(w.buf, p...)
	if len(w.buf) > w.max {
		w.buf = w.buf[len(w.buf)-w.max:]
	}

	return len(p), nil
}

// lastLines returns the last n non-blank lines of s.
func lastLines(s string, n int) string {
	var lines []string
	for _, line := range strings.Split(s, "\n") {
		if line = strings.TrimRight(line, " \t\r"); strings.TrimSpace(line) != "" {
			lines = append(lines, line)
		}
	}
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}

	return strings.Join(lines, "\n")
}
