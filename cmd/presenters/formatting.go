package presenters

import (
	"fmt"
	"math"
	"time"
)

func formatRelativeTime(t time.Time) string {
	dur := time.Since(t)

	if dur.Seconds() < 60 {
		return fmt.Sprintf("%ds ago", int64(dur.Seconds()))
	}
	if dur.Minutes() < 60 {
		return fmt.Sprintf("%dm%ds ago", int64(dur.Minutes()), int64(math.Mod(dur.Seconds(), 60)))
	}

	if dur.Hours() < 24 {
		return fmt.Sprintf("%dh%dm ago", int64(dur.Hours()), int64(math.Mod(dur.Minutes(), 60)))
	}

	return formatTime(t)
}

func formatTime(t time.Time) string {
	return t.Format(time.RFC3339)
}
