// Package format implements various formatters.
package format

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/logrusorgru/aurora"
	"github.com/superfly/flyctl/api"
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

	return t.Format("Jan 2 2006 15:04")
}

// Time is shorthand for t.Format(time.RFC3339).
func Time(t time.Time) string {
	return t.Format(time.RFC3339)
}

func HealthChecksSummary(allocs ...*api.AllocationStatus) string {
	var total, pass, crit, warn int

	for _, alloc := range allocs {
		if n := len(alloc.Checks); n > 0 {
			total += n
			pass += passingChecks(alloc.Checks)
			crit += critChecks(alloc.Checks)
			warn += warnChecks(alloc.Checks)
		}
	}

	if total == 0 {
		return ""
	}

	checkStr := fmt.Sprintf("%d total", total)

	if pass > 0 {
		checkStr += ", " + fmt.Sprintf("%d passing", pass)
	}
	if warn > 0 {
		checkStr += ", " + fmt.Sprintf("%d warning", warn)
	}
	if crit > 0 {
		checkStr += ", " + fmt.Sprintf("%d critical", crit)
	}

	return checkStr
}

func AllocStatus(alloc *api.AllocationStatus) string {
	status := alloc.Status

	if status == "running" {
		for _, c := range alloc.Checks {
			if (c.Name == "role" || c.Name == "status") && c.Status != "" {
				o := strings.TrimSpace(c.Output)
				if len(o) > 12 {
					o = o[:12]
				}

				if o == "" {
					o = "starting"
				}
				status = fmt.Sprintf(
					"%s (%s)",
					status,
					o,
				)
				break

			}
		}
	}

	if alloc.Transitioning {
		return aurora.Bold(status).String()
	}

	return status
}

func passingChecks(checks []api.CheckState) (n int) {
	for _, check := range checks {
		if check.Status == "passing" {
			n++
		}
	}

	return n
}

func warnChecks(checks []api.CheckState) (n int) {
	for _, check := range checks {
		if check.Status == "warning" {
			n++
		}
	}

	return n
}

func critChecks(checks []api.CheckState) (n int) {
	for _, check := range checks {
		if check.Status == "critical" {
			n++
		}
	}

	return n
}
