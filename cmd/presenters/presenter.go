package presenters

import (
	"fmt"
	"io"

	"github.com/olekukonko/tablewriter"
)

type Presentable interface {
	FieldNames() []string
	FieldMap() map[string]string
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
	kvMap := p.Item.FieldMap()

	if !p.Opts.HideHeader {
		table.SetHeader(cols)
	}
	table.SetBorder(false)
	table.SetAutoWrapText(false)

	for _, kv := range p.Item.Records() {
		fields := []string{}
		for _, col := range cols {
			fields = append(fields, kv[kvMap[col]])
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
	kvMap := p.Item.FieldMap()

	table.SetBorder(false)
	table.SetColumnSeparator("=")

	for _, kv := range p.Item.Records() {
		for _, col := range cols {
			table.Append([]string{col, kv[kvMap[col]]})
		}
		table.Render()

		fmt.Println()
	}

	return nil
}
