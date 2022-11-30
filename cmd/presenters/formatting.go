package presenters

import (
	"fmt"
	"math"
	"strings"
	"time"
)

func FormatRelativeTime(t time.Time) string {
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

	return FormatTime(t)
}

func FormatTime(t time.Time) string {
	return t.Format(time.RFC3339)
}

func GetStringInBetweenTwoString(str string, startS string, endS string) (result string, found bool) {
	s := strings.Index(str, startS)
	if s == -1 {
		return result, false
	}
	newS := str[s+len(startS):]
	e := strings.Index(newS, endS)
	if e == -1 {
		return result, false
	}
	result = newS[:e]
	return result, true
}
