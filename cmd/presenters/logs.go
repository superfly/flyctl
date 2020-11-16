package presenters

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/logrusorgru/aurora"
	"github.com/superfly/flyctl/api"
)

type LogPresenter struct {
	RemoveNewlines bool
	HideRegion     bool
	HideAllocID    bool
}

func (lp *LogPresenter) FPrint(w io.Writer, asJSON bool, entries []api.LogEntry) {
	for _, entry := range entries {
		lp.printEntry(w, asJSON, entry)
	}
}

var newLineReplacer = strings.NewReplacer("\r\n", aurora.Faint("↩︎").String(), "\n", aurora.Faint("↩︎").String())
var newline = []byte("\n")

func (lp *LogPresenter) printEntry(w io.Writer, asJSON bool, entry api.LogEntry) {
	if asJSON {
		outBuf, _ := json.MarshalIndent(entry, "", "    ")
		fmt.Fprintln(w, string(outBuf))
		return
	}

	fmt.Fprintf(w, "%s ", aurora.Faint(entry.Timestamp))

	if !lp.HideAllocID {
		fmt.Fprintf(w, "%s ", entry.Instance)
	}

	if !lp.HideRegion {
		fmt.Fprintf(w, "%s ", aurora.Green(entry.Region))
	}

	fmt.Fprintf(w, "[%s] ", aurora.Colorize(entry.Level, levelColor(entry.Level)))

	if lp.RemoveNewlines {
		newLineReplacer.WriteString(w, entry.Message)
	} else {
		w.Write([]byte(entry.Message))
	}

	w.Write(newline)
}

func levelColor(level string) aurora.Color {
	switch level {
	case "debug":
		return aurora.CyanFg
	case "info":
		return aurora.BlueFg
	case "warning":
		return aurora.MagentaFg
	}
	return aurora.RedFg
}
