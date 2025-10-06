package render

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/logrusorgru/aurora"
	"github.com/morikuni/aec"
	"github.com/olekukonko/tablewriter"
	"github.com/superfly/flyctl/iostreams"
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

// NewTable creates and configures a new tablewriter.Table with our default
// settings. The caller can make other configuration changes before calling
// table.Render() on the returned Table.
func NewTable(w io.Writer, title string, rows [][]string, cols ...string) *tablewriter.Table {
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

	return table
}

// Table renders the table defined by the given properties into w. Both title &
// cols are optional.
func Table(w io.Writer, title string, rows [][]string, cols ...string) error {
	table := NewTable(w, title, rows, cols...)

	table.Render()

	fmt.Fprintln(w)

	return nil
}

func VerticalTable(w io.Writer, title string, objects [][]string, cols ...string) error {
	if title != "" {
		fmt.Fprintln(w, aurora.Bold(title))
	}

	table := tablewriter.NewWriter(w)
	table.SetBorder(false)
	table.SetAutoWrapText(false)
	table.SetColumnSeparator("=")
	table.SetAlignment(tablewriter.ALIGN_LEFT)
	table.SetHeaderAlignment(tablewriter.ALIGN_LEFT)

	for _, obj := range objects {
		for i, col := range cols {
			table.Append([]string{col, obj[i]})
		}

		table.Render()

		fmt.Fprintln(w)
	}

	return nil
}

func ReusableTable(w io.Writer, title string, rows [][]string, cols ...string) (err error) {
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

	return
}

func NewTextBlock(ctx context.Context, v ...interface{}) (tb *TextBlock) {
	io := iostreams.FromContext(ctx)
	colorize := io.ColorScheme()

	tb = &TextBlock{
		out:      iostreams.FromContext(ctx).ErrOut,
		colorize: colorize,
	}

	if len(v) > 0 {
		tb.Println(colorize.Green("==> " + fmt.Sprint(v...)))
	}

	return
}

type TextBlock struct {
	out      io.Writer
	colorize *iostreams.ColorScheme
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
	tb.Println("> ", fmt.Sprint(v...))
}

// Detailf prints to the output ctx carries. It behaves similarly to log.Printf.
func (tb *TextBlock) Detailf(format string, v ...interface{}) {
	tb.Detail(fmt.Sprintf(format, v...))
}

func (tb *TextBlock) Overwrite() {
	tb.Print(aec.Up(1), aec.EraseLine(aec.EraseModes.All))
}

func (tb *TextBlock) Done(v ...interface{}) {
	tb.Println(tb.colorize.Green("--> " + fmt.Sprint(v...)))
}

func (tb *TextBlock) Donef(format string, v ...interface{}) {
	tb.Done(fmt.Sprintf(format, v...))
}
