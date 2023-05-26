package tokens

import (
	"context"
	"fmt"
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/command/orgs"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/render"
	"github.com/superfly/flyctl/iostreams"
)

func newList() *cobra.Command {
	const (
		short = "List deploy tokens"
		long  = short
		usage = "list"
	)

	cmd := command.New(usage, short, long, runList,
		command.RequireSession,
		command.LoadAppNameIfPresent,
	)

	flag.Add(cmd,
		flag.App(),
		flag.JSONOutput(),
		flag.String{
			Name:        "scope",
			Shorthand:   "s",
			Description: "either 'app' or 'org'",
			Default:     "app",
		},
	)

	return cmd
}

func runList(ctx context.Context) (err error) {
	apiClient := client.FromContext(ctx).API()
	out := iostreams.FromContext(ctx).Out
	var rows [][]string

	scope := flag.GetString(ctx, "scope")
	switch scope {
	case "app":
		appName := appconfig.NameFromContext(ctx)
		if appName == "" {
			return command.ErrRequireAppName
		}

		app, err := apiClient.GetApp(ctx, appName)
		if err != nil {
			return fmt.Errorf("failed retrieving app %s: %w", appName, err)
		}

		for _, token := range app.LimitedAccessTokens.Nodes {
			rows = append(rows, []string{token.Id, token.Name, token.ExpiresAt.String()})
		}

	case "org":
		org, err := orgs.OrgFromFirstArgOrSelect(ctx, api.AdminOnly)
		if err != nil {
			return fmt.Errorf("failed retrieving org %w", err)
		}

		for _, token := range org.LimitedAccessTokens.Nodes {
			rows = append(rows, []string{token.Id, token.Name, token.ExpiresAt.String()})
		}
	}
	_ = render.Table(out, "", rows, "ID", "Name", "Expires At")
	return nil
}
