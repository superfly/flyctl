package helpers

import (
	"io"

	tablewriter "github.com/olekukonko/tablewriter"
	"github.com/olekukonko/tablewriter/tw"
)

func MakeSimpleTable(out io.Writer, headings []string) (table *tablewriter.Table) {
	newtable := tablewriter.NewTable(out,
		tablewriter.WithHeader(headings),
		tablewriter.WithHeaderAlignment(tw.AlignLeft),
		tablewriter.WithRowAlignment(tw.AlignLeft),
		tablewriter.WithRendition(tw.Rendition{
			Borders: tw.Border{Left: tw.Off, Right: tw.Off, Top: tw.Off, Bottom: tw.Off},
		}),
	)
	newtable.Configure(func(cfg *tablewriter.Config) {
		cfg.Header.Formatting.AutoFormat = tw.On
		cfg.Row.Formatting.AutoWrap = tw.WrapNone
	})
	return newtable
}
