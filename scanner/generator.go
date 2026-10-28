package scanner

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

// How much of a failed generator's stderr goes into its error.
const (
	generatorErrLines = 5
	generatorErrBytes = 4096
)

// runGenerator runs a Dockerfile generator, or the package manager installing
// one, attached to the terminal. Its stderr still reaches the user as it is
// written; on failure the last lines of it are added to the error too, so
// launch telemetry records why it failed instead of just "exit status 1".
// The *exec.ExitError stays wrapped for errors.As.
func runGenerator(cmd *exec.Cmd) error {
	tail := &tailWriter{max: generatorErrBytes}
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = io.MultiWriter(os.Stderr, tail)

	err := cmd.Run()
	if err == nil {
		return nil
	}

	if out := lastLines(string(tail.buf), generatorErrLines); out != "" {
		return fmt.Errorf("%w:\n%s", err, out)
	}

	return err
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
