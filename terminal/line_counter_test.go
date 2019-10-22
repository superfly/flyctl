package terminal

import (
	"fmt"
	"os"
	"testing"

	"gotest.tools/assert"
)

func TestLineCounter(t *testing.T) {
	w := LineCounter{W: os.Stdout}
	fmt.Fprint(&w, "Hello\nWorld\n")
	assert.Equal(t, 2, w.LinesWritten())
	w.Reset()
	assert.Equal(t, 0, w.LinesWritten())
}
