package presenters

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/logrusorgru/aurora"
	"github.com/superfly/flyctl/pkg/logs"
)

type LogPresenter struct {
	RemoveNewlines bool
	HideRegion     bool
	HideAllocID    bool
}

func (lp *LogPresenter) FPrint(w io.Writer, asJSON bool, entry logs.LogEntry) {
	lp.printEntry(w, asJSON, entry)
}

var newLineReplacer = strings.NewReplacer("\r\n", aurora.Faint("↩︎").String(), "\n", aurora.Faint("↩︎").String())
var newline = []byte("\n")

func (lp *LogPresenter) printEntry(w io.Writer, asJSON bool, entry logs.LogEntry) {
	if asJSON {
		outBuf, _ := json.MarshalIndent(entry, "", "    ")
		fmt.Fprintln(w, string(outBuf))
		return
	}

	// parse entry.Timestamp and truncate from nanoseconds to milliseconds
	timestamp, err := time.Parse(time.RFC3339Nano, entry.Timestamp)
	if err != nil {
		fmt.Fprintf(w, "Error parsing timestamp: %s\n", err)
		return
	}

	fmt.Fprintf(w, "%s ", aurora.Faint(timestamp.Format("2006-01-02T15:04:05.000")))

	if !lp.HideAllocID {
		if entry.Meta.Event.Provider != "" {
			if entry.Instance != "" {
				fmt.Fprintf(w, "%s[%s]", entry.Meta.Event.Provider, entry.Instance)
			} else {
				fmt.Fprint(w, entry.Meta.Event.Provider)
			}
		} else if entry.Instance != "" {
			fmt.Fprintf(w, "%s", entry.Instance)
		}
		fmt.Fprint(w, " ")
	}

	if !lp.HideRegion {
		fmt.Fprintf(w, "%s ", aurora.Green(entry.Region))
	}

	fmt.Fprintf(w, "[%s] ", aurora.Colorize(entry.Level, levelColor(entry.Level)))

	printFieldIfPresent(w, "error.code", entry.Meta.Error.Code)
	hadErrorMsg := printFieldIfPresent(w, "error.message", entry.Meta.Error.Message)
	printFieldIfPresent(w, "request.method", entry.Meta.HTTP.Request.Method)
	printFieldIfPresent(w, "request.url", entry.Meta.URL.Full)
	printFieldIfPresent(w, "request.id", entry.Meta.HTTP.Request.ID)
	printFieldIfPresent(w, "response.status", entry.Meta.HTTP.Response.StatusCode)

	if !hadErrorMsg {
		if lp.RemoveNewlines {
			_, _ = newLineReplacer.WriteString(w, entry.Message)
		} else {
			_, _ = w.Write([]byte(entry.Message))
		}
	}

	_, _ = w.Write(newline)
}

func printFieldIfPresent(w io.Writer, name string, value interface{}) bool {
	switch v := value.(type) {
	case string:
		if v != "" {
			fmt.Fprintf(w, `%s"%s" `, aurora.Faint(name+"="), v)
			return true
		}
	case int:
		if v > 0 {
			fmt.Fprintf(w, "%s%d ", aurora.Faint(name+"="), v)
			return true
		}
	}

	return false
}

func levelColor(level string) aurora.Color {
	switch level {
	case "debug":
		return aurora.CyanFg
	case "info":
		return aurora.BlueFg
	case "warn":
	case "warning":
		return aurora.YellowFg
	}
	return aurora.RedFg
}
