package ssh

import (
	"bytes"
	"context"

	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/command/orgs"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/render"
	"github.com/superfly/flyctl/iostreams"
)

func newLog() *cobra.Command {
	const (
		short = `List the SSH certificates issued for an organization`
		long  = `List the SSH certificates that have been issued for an organization,
printing each certificate in full. The organization comes from ` + "`--org`" + `
or the first argument; otherwise you pick one from a list. Use ` + "`--json`" + `
for machine-readable output.`
		usage = "log"
	)

	cmd := command.New(usage, short, long, runLog, command.RequireSession)

	flag.Add(cmd,
		flag.JSONOutput(),
		flag.Org(),
	)

	return cmd
}

func runLog(ctx context.Context) (err error) {
	client := flyutil.ClientFromContext(ctx)
	jsonOutput := config.FromContext(ctx).JSONOutput
	out := iostreams.FromContext(ctx).Out

	org, err := orgs.OrgFromEnvVarOrFirstArgOrSelect(ctx)
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

	table := tablewriter.NewTable(out,
		tablewriter.WithHeader([]string{"Root", "Certificate"}),
	)

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
				table.Append(root, buf.String()) //nolint:errcheck
				if first {
					root = ""
					first = false
				}
				buf.Reset()
			}
		}

		if buf.Len() != 0 {
			table.Append(root, buf.String()) //nolint:errcheck
		}
	}

	table.Render() //nolint:errcheck

	return nil
}
