package render

import (
	"fmt"
	"math"
	"time"
)

func RelativeTime(t time.Time) string {
	if t.Before(time.Now()) {
		dur := time.Since(t)
		if dur.Seconds() < 1 {
			return "just now"
		}
		if dur.Seconds() < 60 {
			return fmt.Sprintf("%ds ago", int64(dur.Seconds()))
		}
		if dur.Minutes() < 60 {
			return fmt.Sprintf("%dm%ds ago", int64(dur.Minutes()), int64(math.Mod(dur.Seconds(), 60)))
		}

		if dur.Hours() < 24 {
			return fmt.Sprintf("%dh%dm ago", int64(dur.Hours()), int64(math.Mod(dur.Minutes(), 60)))
		}
	} else {
		dur := time.Until(t)
		if dur.Seconds() < 60 {
			return fmt.Sprintf("%ds", int64(dur.Seconds()))
		}
		if dur.Minutes() < 60 {
			return fmt.Sprintf("%dm%ds", int64(dur.Minutes()), int64(math.Mod(dur.Seconds(), 60)))
		}

		if dur.Hours() < 24 {
			return fmt.Sprintf("%dh%dm", int64(dur.Hours()), int64(math.Mod(dur.Minutes(), 60)))
		}
	}

	return Time(t)
}

// Time is shorthand for t.Format(time.RFC3339).
func Time(t time.Time) string {
	return t.Format(time.RFC3339)
}
