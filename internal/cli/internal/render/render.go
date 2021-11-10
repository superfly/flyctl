package render

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/logrusorgru/aurora"
	"github.com/olekukonko/tablewriter"
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
