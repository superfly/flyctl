package tokens

import (
	"context"
	"fmt"
	"time"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/gql"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command/orgs"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/render"
	"github.com/superfly/flyctl/iostreams"

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
		newOrg(),
		newLiteFSCloud(),
	)

	return cmd
}

func newOrg() *cobra.Command {
	const (
		short = "Create org deploy tokens"
		long  = "Create an API token limited to managing a single org and its resources. Tokens are valid for 20 years by default. We recommend using a shorter expiry if practical."
		usage = "org"
	)

	cmd := command.New(usage, short, long, runOrg,
		command.RequireSession,
	)

	flag.Add(cmd,
		flag.JSONOutput(),
		flag.Duration{
			Name:        "expiry",
			Shorthand:   "x",
			Description: "The duration that the token will be valid",
			Default:     time.Hour * 24 * 365 * 20,
		},
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
		command.LoadAppNameIfPresent,
	)

	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
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
	)

	return cmd
}

func newLiteFSCloud() *cobra.Command {
	const (
		short = "Create LiteFS Cloud tokens"
		long  = "Create an API token limited to a single LiteFS Cloud cluster."
		usage = "litefs-cloud"
	)

	cmd := command.New(usage, short, long, runLiteFSCloud,
		command.RequireSession,
	)

	flag.Add(cmd,
		flag.JSONOutput(),
		flag.Org(),
		flag.String{
			Name:        "name",
			Shorthand:   "n",
			Description: "Token name",
			Default:     "LiteFS Cloud token",
		},
		flag.Duration{
			Name:        "expiry",
			Shorthand:   "x",
			Description: "The duration that the token will be valid",
			Default:     time.Hour * 24 * 365 * 20,
		},
		flag.String{
			Name:        "cluster",
			Shorthand:   "c",
			Description: "Cluster name",
		},
	)

	return cmd
}

func makeToken(ctx context.Context, apiClient *api.Client, orgID string, expiry string, profile string, options *gql.LimitedAccessTokenOptions) (*gql.CreateLimitedAccessTokenResponse, error) {
	resp, err := gql.CreateLimitedAccessToken(
		ctx,
		apiClient.GenqClient,
		flag.GetString(ctx, "name"),
		orgID,
		profile,
		options,
		expiry,
	)
	if err != nil {
		return nil, fmt.Errorf("failed creating deploy token: %w", err)
	}
	return resp, nil
}

func runOrg(ctx context.Context) error {
	var token string
	apiClient := client.FromContext(ctx).API()

	expiry := ""
	if expiryDuration := flag.GetDuration(ctx, "expiry"); expiryDuration != 0 {
		expiry = expiryDuration.String()
	}

	org, err := orgs.OrgFromEnvVarOrFirstArgOrSelect(ctx)
	if err != nil {
		return fmt.Errorf("failed retrieving org %w", err)
	}

	resp, err := makeToken(ctx, apiClient, org.ID, expiry, "deploy_organization", &gql.LimitedAccessTokenOptions{})
	if err != nil {
		return err
	}

	token = resp.CreateLimitedAccessToken.LimitedAccessToken.TokenHeader

	io := iostreams.FromContext(ctx)
	if config.FromContext(ctx).JSONOutput {
		render.JSON(io.Out, map[string]string{"token": token})
	} else {
		fmt.Fprintln(io.Out, token)
	}

	return nil
}

func runDeploy(ctx context.Context) (err error) {
	var token string
	apiClient := client.FromContext(ctx).API()

	expiry := ""
	if expiryDuration := flag.GetDuration(ctx, "expiry"); expiryDuration != 0 {
		expiry = expiryDuration.String()
	}

	appName := appconfig.NameFromContext(ctx)

	app, err := apiClient.GetAppCompact(ctx, appName)
	if err != nil {
		return fmt.Errorf("failed retrieving app %s: %w", appName, err)
	}

	resp, err := makeToken(ctx, apiClient, app.Organization.ID, expiry, "deploy", &gql.LimitedAccessTokenOptions{
		"app_id": app.ID,
	})
	if err != nil {
		return err
	}

	token = resp.CreateLimitedAccessToken.LimitedAccessToken.TokenHeader

	io := iostreams.FromContext(ctx)
	if config.FromContext(ctx).JSONOutput {
		render.JSON(io.Out, map[string]string{"token": token})
	} else {
		fmt.Fprintln(io.Out, token)
	}

	return nil
}

func runLiteFSCloud(ctx context.Context) (err error) {
	var token string
	apiClient := client.FromContext(ctx).API()

	expiry := ""
	if expiryDuration := flag.GetDuration(ctx, "expiry"); expiryDuration != 0 {
		expiry = expiryDuration.String()
	}

	cluster := flag.GetString(ctx, "cluster")
	if cluster == "" {
		return fmt.Errorf("cluster name is not provided")
	}

	org, err := orgs.OrgFromFlagOrSelect(ctx)
	if err != nil {
		return fmt.Errorf("failed retrieving org %w", err)
	}

	resp, err := makeToken(ctx, apiClient, org.ID, expiry, "litefs_cloud", &gql.LimitedAccessTokenOptions{
		"cluster": cluster,
	})
	if err != nil {
		return err
	}

	token = resp.CreateLimitedAccessToken.LimitedAccessToken.TokenHeader

	io := iostreams.FromContext(ctx)
	if config.FromContext(ctx).JSONOutput {
		render.JSON(io.Out, map[string]string{"token": token})
	} else {
		fmt.Fprintln(io.Out, token)
	}

	return nil
}
