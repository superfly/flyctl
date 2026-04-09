package main

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"

	"github.com/olekukonko/tablewriter"
	"github.com/olekukonko/tablewriter/tw"
)

func newOutput(w io.Writer, format string) output {
	switch format {
	case "csv":
		return NewCSVOutput(w)
	default:
		return NewTableOutput(w)
	}
}

type output interface {
	SetHeader([]string)
	Append([]string)
	Flush() error
}

func NewTableOutput(w io.Writer) *tableOutput {
	t := tablewriter.NewTable(w,
		tablewriter.WithRendition(tw.Rendition{
			Borders: tw.Border{Left: tw.Off, Right: tw.Off, Top: tw.Off, Bottom: tw.Off},
		}),
	)
	t.Configure(func(cfg *tablewriter.Config) {
		cfg.Row.Formatting.AutoWrap = tw.WrapNone
	})

	return &tableOutput{
		table: t,
	}
}

type tableOutput struct {
	table *tablewriter.Table
}

func (t *tableOutput) SetHeader(header []string) {
	t.table.Options(tablewriter.WithHeader(header))
}

func (t *tableOutput) Append(row []string) {
	args := make([]interface{}, len(row))
	for i, v := range row {
		args[i] = v
	}
	t.table.Append(args...) //nolint:errcheck
}

func (t *tableOutput) Flush() error {
	return t.table.Render()
}

func NewCSVOutput(w io.Writer) *csvOutput {
	return &csvOutput{
		w: csv.NewWriter(w),
	}
}

type csvOutput struct {
	w *csv.Writer
}

func (t *csvOutput) SetHeader(header []string) {
	t.Append(header)
}

func (t *csvOutput) Append(row []string) {
	if err := t.w.Write(row); err != nil {
		panic(err)
	}
}

func (t *csvOutput) Flush() error {
	t.w.Flush()

	return t.w.Error()
}

func prettyPrintJSON(v any) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		panic(err)
	}
	fmt.Println(string(b))
}
