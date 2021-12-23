package certs

import (
	"context"
	"fmt"

	"github.com/dustin/go-humanize"
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/cli/internal/app"
	"github.com/superfly/flyctl/internal/cli/internal/command"
	"github.com/superfly/flyctl/internal/cli/internal/config"
	"github.com/superfly/flyctl/internal/cli/internal/flag"
	"github.com/superfly/flyctl/internal/cli/internal/render"
	"github.com/superfly/flyctl/internal/client"
	"github.com/superfly/flyctl/pkg/iostreams"
)

func newList() *cobra.Command {
	const (
		long = `The LIST command will list all currently registered certificates.
		`
		short = "List certificates"
	)

	list := command.New("list", short, long, RunList, command.RequireSession, command.RequireAppName)

	flag.Add(list,
		flag.App(),
		flag.AppConfig(),
	)

	return list
}

func RunList(ctx context.Context) (err error) {
	app := app.NameFromContext(ctx)

	client := client.FromContext(ctx).API()

	certs, err := client.GetAppCertificates(ctx, app)
	if err != nil {
		return fmt.Errorf("failed to list certificates: %w", err)
	}

	out := iostreams.FromContext(ctx).Out
	if cfg := config.FromContext(ctx); cfg.JSONOutput {
		_ = render.JSON(out, certs)

		return
	}

	rows := make([][]string, 0, len(certs))
	for _, cert := range certs {
		rows = append(rows, []string{
			cert.Hostname,
			humanize.Time(cert.CreatedAt),
			cert.ClientStatus,
		})

	}

	_ = render.Table(out, "", rows, "Hostname", "Added", "Status")

	return nil
}
