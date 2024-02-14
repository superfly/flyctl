package statuslogger

import "fmt"

type Status int

const (
	StatusNone Status = iota
	StatusRunning
	StatusSuccess
	StatusFailure
)

const (
	glyphNone    = " "
	glyphRunning = ">"
	glyphSuccess = "✔"
	glyphFailure = "✖"
)

var glyphsRunning = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

func (status Status) charFor(frame int) string {
	switch status {
	case StatusNone:
		return glyphNone
	case StatusRunning:
		if frame == -1 {
			return glyphRunning
		}
		return glyphsRunning[frame]
	case StatusSuccess:
		return glyphSuccess
	case StatusFailure:
		return glyphFailure
	default:
		return "?"
	}
}

func formatIndex(n, total int) string {
	pad := 0
	for i := total; i != 0; i /= 10 {
		pad++
	}
	return fmt.Sprintf("[%0*d/%d]", pad, n+1, total)
}
