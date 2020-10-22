package helpers

import (
	"io"

	tablewriter "github.com/olekukonko/tablewriter"
)

func MakeSimpleTable(out io.Writer, headings []string) (table *tablewriter.Table) {
	newtable := tablewriter.NewWriter(out)
	newtable.SetHeader(headings)
	newtable.SetHeaderAlignment(tablewriter.ALIGN_LEFT)
	newtable.SetAlignment(tablewriter.ALIGN_LEFT)
	newtable.SetNoWhiteSpace(true)
	newtable.SetTablePadding(" ")
	newtable.SetCenterSeparator("")
	newtable.SetColumnSeparator("")
	newtable.SetRowSeparator("")
	return newtable
}
