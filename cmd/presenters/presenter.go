package presenters

import (
	"fmt"
	"io"

	"github.com/olekukonko/tablewriter"
)

type Presentable interface {
	FieldNames() []string
	Records() []map[string]string
}

type Presenter struct {
	Item Presentable
	Out  io.Writer
	Opts Options
}

type Options struct {
	Vertical   bool
	HideHeader bool
}

func (p *Presenter) Render() error {
	if p.Opts.Vertical {
		return p.renderFieldList()
	}

	return p.renderTable()
}

func (p *Presenter) renderTable() error {
	table := tablewriter.NewWriter(p.Out)

	cols := p.Item.FieldNames()

	if !p.Opts.HideHeader {
		table.SetHeader(cols)
	}
	table.SetBorder(false)
	table.SetHeaderLine(false)
	table.SetAutoWrapText(false)
	table.SetAlignment(tablewriter.ALIGN_LEFT)
	table.SetHeaderAlignment(tablewriter.ALIGN_LEFT)
	table.SetColumnSeparator(" ")

	for _, kv := range p.Item.Records() {
		fields := []string{}
		for _, col := range cols {
			fields = append(fields, kv[col])
		}
		table.Append(fields)
	}

	table.Render()

	fmt.Println()

	return nil
}

func (p *Presenter) renderFieldList() error {
	table := tablewriter.NewWriter(p.Out)

	cols := p.Item.FieldNames()

	table.SetBorder(false)
	table.SetAutoWrapText(false)
	table.SetColumnSeparator("=")
	table.SetColumnAlignment([]int{tablewriter.ALIGN_DEFAULT, tablewriter.ALIGN_LEFT})

	for _, kv := range p.Item.Records() {
		for _, col := range cols {
			table.Append([]string{col, kv[col]})
		}
		table.Render()

		fmt.Println()
	}

	return nil
}
