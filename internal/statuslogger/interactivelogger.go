package statuslogger

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/morikuni/aec"
	"github.com/superfly/flyctl/iostreams"
)

type interactiveLogger struct {
	lock        sync.Mutex
	io          *iostreams.IOStreams
	statusFrame int
	showStatus  bool

	running bool

	lines []*interactiveLine

	// Should we include an item prefix, such as [01/10]?
	logNumbers bool
}

func (sl *interactiveLogger) Line(i int) StatusLine {
	return sl.lines[i]
}

const (
	divider = "-------"
)

func (sl *interactiveLogger) Destroy(clear bool) {
	sl.lock.Lock()
	defer sl.lock.Unlock()

	sl.running = false

	// The +2 is to account for the divider before jobs
	numLines := 2 + len(sl.lines)

	if clear {
		buf := strings.Repeat(aec.EraseLine(aec.EraseModes.All).String()+"\n", numLines)
		buf += aec.Up(uint(numLines)).String()
		fmt.Fprint(sl.io.Out, buf)
	} else {
		fmt.Fprintf(sl.io.Out, "%s%s\n", aec.Down(uint(numLines)), divider)
	}
}

func (sl *interactiveLogger) animateThread() {
	// Increment the animation frame every 2 iterations
	// Each iteration is 50ms, so this is 100ms per frame

	// We redraw so often in order to chase the beam, so to speak
	// If three lines of text are drawn between frames, our status block will
	// bleed into those new lines. Redrawing, along with the two lines of whitespace,
	// are generally enough to prevent this.
	incrementAnim := 0
	for {
		sl.lock.Lock()
		if !sl.running {
			sl.lock.Unlock()
			return
		}
		if sl.showStatus {
			incrementAnim += 1
			if incrementAnim == 2 {
				sl.statusFrame = (sl.statusFrame + 1) % len(glyphsRunning)
				incrementAnim = 0
			}
		}
		sl.lockedDraw()
		sl.lock.Unlock()
		time.Sleep(50 * time.Millisecond)
	}
}

func (sl *interactiveLogger) lockedDraw() {

	// Abort if done
	if !sl.running {
		return
	}

	// Draw the entire status block, clearing each row to prevent overwriting
	erase := aec.EraseLine(aec.EraseModes.All).String()
	buf := fmt.Sprintf("%s\n%s%s\n", erase, erase, divider)
	for i, line := range sl.lines {
		buf += erase
		buf += " "
		if sl.showStatus {
			buf += line.status.charFor(sl.statusFrame) + " "
		}
		if sl.logNumbers {
			buf += formatIndex(i, len(sl.lines)) + " "
		}
		buf += line.buf + "\n"
	}
	// Send the cursor back up above the status block
	newlines := strings.Count(buf, "\n")
	buf += aec.Up(uint(newlines)).String()
	fmt.Fprint(sl.io.Out, buf)
}
