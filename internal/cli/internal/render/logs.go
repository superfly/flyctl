package render

import (
	"bytes"
	"fmt"
	"io"
	"time"

	"github.com/logrusorgru/aurora"

	"github.com/superfly/flyctl/pkg/logs"

	"github.com/superfly/flyctl/internal/cli/internal/format"
)

type LogOptions struct {
	RemoveNewlines bool
	HideRegion     bool
	HideAllocID    bool
	JSONFormat     bool
}

// LogOption is a func type that returns a LogOption.
type LogOption func(o *LogOptions)

// RemoveNewlines removes newlines from the log output.
func RemoveNewlines() LogOption {
	return func(o *LogOptions) {
		o.RemoveNewlines = true
	}
}

// HideRegion removes the region from the log output.
func HideRegion() LogOption {
	return func(o *LogOptions) {
		o.HideRegion = true
	}
}

// HideAllocID removes the allocation ID from the log output.
func HideAllocID() LogOption {
	return func(o *LogOptions) {
		o.HideAllocID = true
	}
}

func JSONFormat(json bool) LogOption {
	return func(o *LogOptions) {
		o.JSONFormat = json
	}
}

func LogEntry(w io.Writer, entry logs.LogEntry, opts ...LogOption) (err error) {
	options := &LogOptions{}
	for _, opt := range opts {
		opt(options)
	}

	if options.JSONFormat {
		return JSON(w, entry)
	}

	var ts time.Time
	if ts, err = time.Parse(time.RFC3339Nano, entry.Timestamp); err != nil {
		err = fmt.Errorf("failed parsing timestamp %q: %w", entry.Timestamp, err)

		return
	}

	if !options.HideAllocID {
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

	if !options.HideRegion {
		fmt.Fprintf(w, "%s ", aurora.Green(entry.Region))
	}

	var buf bytes.Buffer
	fmt.Fprintf(&buf, "%s ", aurora.Faint(format.Time(ts)))

	if entry.Meta.Event.Provider != "" {
		if entry.Instance != "" {
			fmt.Fprintf(&buf, "%s[%s]", entry.Meta.Event.Provider, entry.Instance)
		} else {
			fmt.Fprint(&buf, entry.Meta.Event.Provider)
		}
	} else if entry.Instance != "" {
		fmt.Fprintf(&buf, "%s", entry.Instance)
	}

	fmt.Fprintf(&buf, " %s [%s]", aurora.Green(entry.Region), aurora.Colorize(entry.Level, levelColor(entry.Level)))

	printFieldIfPresent(&buf, "error.code", entry.Meta.Error.Code)
	hadErrorMsg := printFieldIfPresent(w, "error.message", entry.Meta.Error.Message)
	printFieldIfPresent(&buf, "request.method", entry.Meta.HTTP.Request.Method)
	printFieldIfPresent(&buf, "request.url", entry.Meta.URL.Full)
	printFieldIfPresent(&buf, "request.id", entry.Meta.HTTP.Request.ID)
	printFieldIfPresent(&buf, "response.status", entry.Meta.HTTP.Response.StatusCode)

	if !hadErrorMsg {
		buf.Write([]byte(entry.Message))
	}

	buf.WriteByte('\n')

	_, err = buf.WriteTo(w)
	return err
}

func printFieldIfPresent(w io.Writer, name string, value interface{}) (present bool) {
	switch v := value.(type) {
	case string:
		if v != "" {
			fmt.Fprintf(w, `%s"%s" `, aurora.Faint(name+"="), v)

			present = true
		}
	case int:
		if v > 0 {
			fmt.Fprintf(w, "%s%d ", aurora.Faint(name+"="), v)

			present = true
		}
	}

	return
}

func levelColor(level string) aurora.Color {
	switch level {
	default:
		return aurora.RedFg
	case "debug":
		return aurora.CyanFg
	case "info":
		return aurora.BlueFg
	case "warn", "warning":
		return aurora.YellowFg
	}
}
