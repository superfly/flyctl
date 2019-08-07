package presenters

import (
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
}

func (p *Presenter) Render() error {
	table := tablewriter.NewWriter(p.Out)

	cols := p.Item.FieldNames()
	kvMap := p.Item.FieldMap()

	table.SetHeader(cols)
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

	return nil
}
