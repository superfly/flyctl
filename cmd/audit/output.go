package main

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"

	"github.com/olekukonko/tablewriter"
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
	t := tablewriter.NewWriter(w)
	t.SetBorder(false)
	t.SetAutoWrapText(false)
	return &tableOutput{
		Table: t,
	}
}

type tableOutput struct {
	*tablewriter.Table
}

func (t *tableOutput) Flush() error {
	t.Table.Render()
	return nil
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
