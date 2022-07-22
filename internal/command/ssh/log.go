package ssh

import (
	"bytes"
	"context"

	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/command/orgs"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/render"
	"github.com/superfly/flyctl/iostreams"
)

func newLog() *cobra.Command {
	const (
		long = `Log of all issued SSH certs
`
		short = long
		usage = "log"
	)

	cmd := command.New(usage, short, long, runLog, command.RequireSession)

	flag.Add(cmd,
		flag.Org(),
	)

	return cmd
}

func runLog(ctx context.Context) (err error) {
	client := client.FromContext(ctx).API()
	jsonOutput := config.FromContext(ctx).JSONOutput
	out := iostreams.FromContext(ctx).Out

	org, err := orgs.OrgFromFirstArgOrSelect(ctx)

	if err != nil {
		return err
	}

	certs, err := client.GetLoggedCertificates(ctx, org.Slug)
	if err != nil {
		return err
	}

	if jsonOutput {
		render.JSON(out, certs)
		return nil
	}

	table := tablewriter.NewWriter(out)

	table.SetHeader([]string{
		"Root",
		"Certificate",
	})

	for _, cert := range certs {
		root := "no"
		if cert.Root {
			root = "yes"
		}

		first := true
		buf := &bytes.Buffer{}
		for i, ch := range cert.Cert {
			buf.WriteRune(ch)
			if i%60 == 0 && i != 0 {
				table.Append([]string{root, buf.String()})
				if first {
					root = ""
					first = false
				}
				buf.Reset()
			}
		}

		if buf.Len() != 0 {
			table.Append([]string{root, buf.String()})
		}
	}

	table.Render()

	return nil
}
