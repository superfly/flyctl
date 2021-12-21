package render

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/briandowns/spinner"
	"github.com/logrusorgru/aurora"
	"github.com/morikuni/aec"
	"github.com/olekukonko/tablewriter"
	"github.com/superfly/flyctl/pkg/iostreams"
)

func JSON(w io.Writer, v interface{}) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "    ")
	return enc.Encode(v)
}

func TitledJSON(w io.Writer, title string, v interface{}) error {
	return JSON(w, map[string]interface{}{
		title: v,
	})
}

// Table renders the table defined by the given properties into w. Both title &
// cols are optional.
func Table(w io.Writer, title string, rows [][]string, cols ...string) error {
	if title != "" {
		fmt.Fprintln(w, aurora.Bold(title))
	}

	table := tablewriter.NewWriter(w)

	if len(cols) > 0 {
		table.SetHeader(cols)
	}

	table.SetBorder(false)
	table.SetHeaderLine(false)
	table.SetAutoWrapText(false)
	table.SetAutoFormatHeaders(true)
	table.SetAlignment(tablewriter.ALIGN_LEFT)
	table.SetHeaderAlignment(tablewriter.ALIGN_LEFT)
	table.SetColumnSeparator(" ")
	table.SetNoWhiteSpace(true)
	table.SetTablePadding("\t")

	table.AppendBulk(rows)

	table.Render()

	fmt.Fprintln(w)

	return nil
}

func NewTextBlock(ctx context.Context, v ...interface{}) (tb *TextBlock) {
	var buf bytes.Buffer

	tb = &TextBlock{
		out: &buf,
	}

	if len(v) > 0 {
		s := spinner.New(spinner.CharSets[11], 100*time.Millisecond)
		tb.spinner = s
		tb.startText = fmt.Sprint(v...)
		s.Suffix = fmt.Sprintf(" %s", tb.startText)
		s.FinalMSG = fmt.Sprintf("%s %s\n", aurora.Green("✓"), tb.startText)
		s.Writer = iostreams.FromContext(ctx).Out
		s.Color("green")
		s.Start()
	}

	return
}

type TextBlock struct {
	out       io.Writer
	spinner   *spinner.Spinner
	startText string
}

func (tb *TextBlock) Print(v ...interface{}) {
	fmt.Fprint(tb.out, v...)
}

func (tb *TextBlock) Println(v ...interface{}) {
	fmt.Fprintln(tb.out, v...)
}

func (tb *TextBlock) Printf(format string, v ...interface{}) {
	fmt.Fprintf(tb.out, format, v...)
}

// Detail prints to the output ctx carries. It behaves similarly to log.Print.
func (tb *TextBlock) Detail(v ...interface{}) {
	tb.Println(aurora.Faint(fmt.Sprint(v...)))
}

// Detailf prints to the output ctx carries. It behaves similarly to log.Printf.
func (tb *TextBlock) Detailf(format string, v ...interface{}) {
	tb.Detail(fmt.Sprintf(format, v...))
}

func (tb *TextBlock) Overwrite() {
	tb.Println(aec.Up(1))
	tb.Println(aec.EraseLine(aec.EraseModes.All))
}

func (tb *TextBlock) Done(v ...interface{}) {
	tb.spinner.Stop()
}

func (tb *TextBlock) Donef(format string, v ...interface{}) {
	tb.Done(fmt.Sprintf(format, v...))
}
