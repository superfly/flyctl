package statuslogger

import (
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/morikuni/aec"
	"github.com/superfly/flyctl/internal/cmdutil"
	"github.com/superfly/flyctl/iostreams"
	"golang.org/x/crypto/ssh/terminal"
)

type interactiveLogger struct {
	lock        sync.Mutex
	io          *iostreams.IOStreams
	statusFrame int
	showStatus  bool

	active bool
	done   bool

	lines     []*interactiveLine
	prevLines int

	// Should we include an item prefix, such as [01/10]?
	logNumbers bool
}

func (il *interactiveLogger) Line(i int) StatusLine {
	return il.lines[i]
}

const (
	divider           = "-------"
	paddingBeforeJobs = 2
)

func (il *interactiveLogger) Destroy(clear bool) {
	il.lock.Lock()
	defer il.lock.Unlock()

	if il.done {
		return
	}

	il.active = false
	il.done = false

	if clear {
		fmt.Print(il.io.Out, il.clearStr())
	} else {
		fmt.Fprintf(il.io.Out, "%s%s\n", aec.Down(uint(il.height(il.prevLines))), divider)
	}
}

func (il *interactiveLogger) consoleSize() (int, int) {
	width, height, err := terminal.GetSize(int(il.io.StdoutFd()))
	if err != nil {
		height = 24
		width = 80
	}
	return width, height
}

// The current sorting algorithm prioritizes failures, in-progress jobs, and then completed jobs.
// It will pick the most recently modified jobs, sequentially in these categories, then finally sort them all by job ID
func (il *interactiveLogger) currentLines(conHeight int) (finalLines []interactiveLine) {

	maxHeight := conHeight - paddingBeforeJobs - 1
	if maxHeight < 0 {
		return nil
	}

	var errorLines []interactiveLine
	var inProgressLines []interactiveLine
	var doneLines []interactiveLine

	// TODO: There's probably a more efficient way to insert these *and* have them sorted at the same time.

	// Give tasks that are done a grace period before they're cleared.
	now := time.Now()
	twoSecondsAgo := now.Add(-time.Second * 2)

	for _, line := range il.lines {
		if line.status == StatusFailure {
			errorLines = append(errorLines, *line)
		} else if line.status == StatusSuccess {
			if line.doneTime.Before(twoSecondsAgo) {
				doneLines = append(doneLines, *line)
			} else {
				// Hack to ensure that this line is still visible
				inProgressLines = append(inProgressLines, *line)
			}
		} else {
			inProgressLines = append(inProgressLines, *line)
		}
	}

	// Intentionally reversed, so that we sort in descending order
	sortByTime := func(a, b interactiveLine) int {
		return b.lastChanged.Compare(a.lastChanged)
	}
	sortById := func(a, b interactiveLine) int {
		return a.lineNum - b.lineNum
	}

	slices.SortStableFunc(errorLines, sortByTime)
	slices.SortStableFunc(inProgressLines, sortByTime)
	slices.SortStableFunc(doneLines, sortByTime)

	defer func() {
		slices.SortFunc(finalLines, sortById)
	}()

	for _, line := range errorLines {
		finalLines = append(finalLines, line)
		if len(finalLines) >= maxHeight {
			return finalLines
		}
	}
	for _, line := range inProgressLines {
		finalLines = append(finalLines, line)
		if len(finalLines) >= maxHeight {
			return finalLines
		}
	}
	for _, line := range doneLines {
		finalLines = append(finalLines, line)
		if len(finalLines) >= maxHeight {
			return finalLines
		}
	}
	return finalLines
}

func (il *interactiveLogger) height(numEntries int) int {

	// The +2 is to account for the divider before jobs
	return paddingBeforeJobs + numEntries
}

func (il *interactiveLogger) clearStr() string {

	total := il.height(il.prevLines)

	return strings.Repeat(aec.EraseLine(aec.EraseModes.All).String()+"\n", total) + aec.Up(uint(total)).String()
}

func (il *interactiveLogger) animateThread() {
	// Increment the animation frame every 2 iterations
	// Each iteration is 50ms, so this is 100ms per frame

	// We redraw so often in order to chase the beam, so to speak
	// If three lines of text are drawn between frames, our status block will
	// bleed into those new lines. Redrawing, along with the two lines of whitespace,
	// are generally enough to prevent this.
	incrementAnim := 0
	for {
		il.lock.Lock()
		if il.done {
			il.lock.Unlock()
			return
		}
		if il.active {
			if il.showStatus {
				incrementAnim += 1
				if incrementAnim == 2 {
					il.statusFrame = (il.statusFrame + 1) % len(glyphsRunning)
					incrementAnim = 0
				}
			}
			il.lockedDraw()
		}
		il.lock.Unlock()
		time.Sleep(50 * time.Millisecond)
	}
}

func (il *interactiveLogger) lockedDraw() {

	if !il.active || il.done {
		return
	}

	conW, conH := il.consoleSize()

	currentLines := il.currentLines(conH)
	if len(currentLines) == 0 {
		return
	}
	defer func() {
		il.prevLines = len(currentLines)
	}()

	// Draw the entire status block, clearing each row to prevent overwriting
	buf := fmt.Sprintf("%s\n%s\n", il.clearStr(), divider)
	for _, line := range currentLines {
		buf += " "
		line_len := 0
		if il.showStatus {
			buf += line.status.charFor(il.statusFrame) + " "
			line_len += 2
		}
		if il.logNumbers {
			idx := formatIndex(line.lineNum, len(il.lines))
			buf += idx + " "
			line_len += len(idx) + 1
		}
		remainingSpace := conW - line_len - 3
		switch {
		case remainingSpace < 0:
		case len(cmdutil.StripANSI(line.buf)) >= remainingSpace:
			buf += substrIgnoreAnsi(line.buf, remainingSpace-1) + "…"
		default:
			buf += line.buf
		}
		buf += "\n"
	}
	// Erase last line, prevent weird display bug
	buf += aec.EraseLine(aec.EraseModes.All).String()
	// Send the cursor back up above the status block
	buf += aec.Up(uint(il.height(len(currentLines)))).String()
	fmt.Fprint(il.io.Out, buf)
}

func substrIgnoreAnsi(str string, length int) string {
	for i := 0; i < length; i++ {
		// Check if ansi escape
		if str[i] == '\x1b' {
			if len(str) > i+2 && str[i+1] == '[' {
				// find next m
				mIdx := strings.IndexByte(str[i+1:], 'm')
				if mIdx != -1 {
					// I don't understand how this could ever be >len(str), but it was
					// causing crashes, so we'll just be safe :)
					length = min(length+mIdx+2, len(str))
					i += mIdx + 1
				}
			}
		}
	}
	return str[:length] + aec.Reset
}

func (il *interactiveLogger) Pause() ResumeFn {
	il.lock.Lock()
	defer il.lock.Unlock()

	fmt.Fprint(il.io.Out, il.clearStr())
	il.active = false

	return func() {
		il.lock.Lock()
		defer il.lock.Unlock()

		il.active = true
		il.lockedDraw()
	}
}
