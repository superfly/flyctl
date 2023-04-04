package tokens

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/gql"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/prompt"
	"github.com/superfly/flyctl/internal/render"
	"github.com/superfly/flyctl/iostreams"
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
		newLogs(),
	)

	return cmd
}

func newDeploy() *cobra.Command {
	const (
		short = "Create deploy tokens"
		long  = "Create an API token limited to managing a single app and its resources. Also available as TOKENS DEPLOY"
		usage = "deploy"
	)

	cmd := command.New(usage, short, long, runDeploy,
		command.RequireSession,
		command.RequireAppName,
	)

	flag.Add(cmd,
		flag.App(),
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
			Default:     time.Hour,
		},
	)

	return cmd
}

func runDeploy(ctx context.Context) (err error) {
	appName := appconfig.NameFromContext(ctx)
	apiClient := client.FromContext(ctx).API()

	app, err := apiClient.GetAppCompact(ctx, appName)
	if err != nil {
		return fmt.Errorf("failed retrieving app %s: %w", appName, err)
	}

	expiry := ""
	if expiryDuration := flag.GetDuration(ctx, "expiry"); expiryDuration != 0 {
		expiry = expiryDuration.String()
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

	io := iostreams.FromContext(ctx)
	if config.FromContext(ctx).JSONOutput {
		render.JSON(io.Out, map[string]string{"token": resp.CreateLimitedAccessToken.LimitedAccessToken.TokenHeader})
	} else {
		fmt.Fprintln(io.Out, resp.CreateLimitedAccessToken.LimitedAccessToken.TokenHeader)
	}

	return nil
}

func newLogs() *cobra.Command {
	const (
		short = "Create log-access tokens"
		long  = "Create an API token limited to accessing logs for an organization or app. Also available as TOKENS LOGS"
		usage = "logs"
	)

	cmd := command.New(usage, short, long, runLogs,
		command.RequireSession,
		command.LoadAppNameIfPresent,
	)

	flag.Add(cmd,
		flag.App(),
		flag.Org(),
		flag.String{
			Name:        "name",
			Shorthand:   "n",
			Description: "Token name",
			Default:     "flyctl logs token",
		},
		flag.Duration{
			Name:        "expiry",
			Shorthand:   "x",
			Description: "The duration that the token will be valid",
			Default:     time.Hour,
		},
	)

	return cmd
}

func runLogs(ctx context.Context) (err error) {
	var (
		apiClient = client.FromContext(ctx).API()
		org       api.OrganizationImpl
		opts      *gql.LimitedAccessTokenOptions
	)

	if appName := appconfig.NameFromContext(ctx); appName != "" {
		app, err := apiClient.GetAppCompact(ctx, appName)
		if err != nil {
			return fmt.Errorf("failed retrieving app %s: %w", appName, err)
		}
		org = app.Organization
		opts = &gql.LimitedAccessTokenOptions{
			"app_ids": []string{app.ID},
		}
	} else {
		if org, err = prompt.Org(ctx); err != nil {
			return
		}
		opts = &gql.LimitedAccessTokenOptions{}
	}

	expiry := ""
	if expiryDuration := flag.GetDuration(ctx, "expiry"); expiryDuration != 0 {
		expiry = expiryDuration.String()
	}

	resp, err := gql.CreateLimitedAccessToken(
		ctx,
		apiClient.GenqClient,
		flag.GetString(ctx, "name"),
		org.GetID(),
		"read_organization_apps",
		opts,
		expiry,
	)
	if err != nil {
		return fmt.Errorf("failed creating log-access token: %w", err)
	}

	io := iostreams.FromContext(ctx)
	if config.FromContext(ctx).JSONOutput {
		render.JSON(io.Out, map[string]string{"token": resp.CreateLimitedAccessToken.LimitedAccessToken.TokenHeader})
	} else {
		fmt.Fprintln(io.Out, resp.CreateLimitedAccessToken.LimitedAccessToken.TokenHeader)
	}

	return nil
}
