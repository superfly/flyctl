package logs

import (
	"context"
	"fmt"

	"github.com/skratchdot/open-golang/open"
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/gql"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/iostreams"
)

func newDashboard() *cobra.Command {

	const (
		short = "View and analyze logs in a web browser"
		long  = short + "\n"
		usage = "dashboard <org_slug>"
	)

	cmd := command.New("dashboard", short, long, runDashboard, command.RequireSession, command.RequireAppName)

	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
	)
	cmd.Args = cobra.NoArgs
	return cmd
}

func runDashboard(ctx context.Context) (err error) {
	var (
		io     = iostreams.FromContext(ctx)
		client = client.FromContext(ctx).API().GenqClient
	)
	appName := appconfig.NameFromContext(ctx)
	appResult, err := gql.GetApp(ctx, client, appName)

	if err != nil {
		return err
	}

	addOnResult, err := gql.GetAddOn(ctx, client, appResult.App.Organization.RawSlug+"-log-shipper")

	if err != nil {
		fmt.Fprintf(io.ErrOut, "You haven't added a logging integration for the %s organization. Set one up with 'flyctl logs shipper setup %s'.\n", appResult.App.Organization.Slug, appResult.App.Organization.Slug)
		return nil
	}

	url := addOnResult.AddOn.SsoLink
	fmt.Fprintf(io.Out, "Opening %s ...\n", url)

	if err := open.Run(url); err != nil {
		return fmt.Errorf("failed opening %s: %w", url, err)
	}

	return
}
