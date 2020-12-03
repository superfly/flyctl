package helpers

import (
	"io"

	tablewriter "github.com/olekukonko/tablewriter"
)

func MakeSimpleTable(out io.Writer, headings []string) (table *tablewriter.Table) {
	newtable := tablewriter.NewWriter(out)
	// Future code to turn headers bold
	// headercolors := []tablewriter.Colors{}
	// for range headings {
	// 	headercolors = append(headercolors, tablewriter.Colors{tablewriter.Bold})
	// }
	newtable.SetHeader(headings)
	newtable.SetHeaderLine(true)
	newtable.SetBorder(false)
	newtable.SetAutoFormatHeaders(false)
	//newtable.SetHeaderColor(headercolors...)
	newtable.SetHeaderAlignment(tablewriter.ALIGN_LEFT)
	newtable.SetAlignment(tablewriter.ALIGN_LEFT)
	newtable.SetNoWhiteSpace(true)
	newtable.SetTablePadding(" ")
	newtable.SetCenterSeparator("")
	newtable.SetColumnSeparator("")
	newtable.SetRowSeparator("")
	return newtable
}
