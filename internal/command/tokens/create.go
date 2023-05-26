package tokens

import (
	"context"
	"fmt"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/gql"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command/orgs"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/render"
	"github.com/superfly/flyctl/iostreams"
	"time"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
)

func newCreate() *cobra.Command {
	const (
		short = "Create Fly.io API tokens"
		long  = "Create Fly.io API tokens"
		usage = "create"
	)

	cmd := command.New(usage, short, long, nil)

	cmd.AddCommand(
		newDeploy(),
	)

	return cmd
}

func newDeploy() *cobra.Command {
	const (
		short = "Create deploy tokens"
		long  = "Create an API token limited to managing a single app and its resources. Also available as TOKENS DEPLOY. Tokens are valid for 20 years by default. We recommend using a shorter expiry if practical."
		usage = "deploy"
	)

	cmd := command.New(usage, short, long, runDeploy,
		command.RequireSession,
	)

	flag.Add(cmd,
		flag.App(),
		flag.JSONOutput(),
		flag.String{
			Name:        "name",
			Shorthand:   "n",
			Description: "Token name",
			Default:     "flyctl deploy token",
		},
		flag.Duration{
			Name:        "expiry",
			Shorthand:   "x",
			Description: "The duration that the token will be valid",
			Default:     time.Hour * 24 * 365 * 20,
		},
		flag.String{
			Name:        "scope",
			Shorthand:   "s",
			Description: "either 'app' or 'org'",
			Default:     "app",
		},
	)

	return cmd
}

func runDeploy(ctx context.Context) (err error) {
	var token string
	apiClient := client.FromContext(ctx).API()

	expiry := ""
	if expiryDuration := flag.GetDuration(ctx, "expiry"); expiryDuration != 0 {
		expiry = expiryDuration.String()
	}

	scope := flag.GetString(ctx, "scope")
	switch scope {
	case "app":
		appName := appconfig.NameFromContext(ctx)

		app, err := apiClient.GetAppCompact(ctx, appName)
		if err != nil {
			return fmt.Errorf("failed retrieving app %s: %w", appName, err)
		}

		resp, err := gql.CreateLimitedAccessToken(
			ctx,
			apiClient.GenqClient,
			flag.GetString(ctx, "name"),
			app.Organization.ID,
			"deploy",
			&gql.LimitedAccessTokenOptions{
				"app_id": app.ID,
			},
			expiry,
		)
		if err != nil {
			return fmt.Errorf("failed creating deploy token: %w", err)
		}

		token = resp.CreateLimitedAccessToken.LimitedAccessToken.TokenHeader
	case "org":
		org, err := orgs.OrgFromFirstArgOrSelect(ctx, api.AdminOnly)
		if err != nil {
			return fmt.Errorf("failed retrieving org %w", err)
		}

		resp, err := gql.CreateLimitedAccessToken(
			ctx,
			apiClient.GenqClient,
			flag.GetString(ctx, "name"),
			org.ID,
			"deploy_organization",
			&gql.LimitedAccessTokenOptions{},
			expiry,
		)
		if err != nil {
			return fmt.Errorf("failed creating deploy token: %w", err)
		}

		token = resp.CreateLimitedAccessToken.LimitedAccessToken.TokenHeader
	}

	io := iostreams.FromContext(ctx)
	if config.FromContext(ctx).JSONOutput {
		render.JSON(io.Out, map[string]string{"token": token})
	} else {
		fmt.Fprintln(io.Out, token)
	}

	return nil
}
