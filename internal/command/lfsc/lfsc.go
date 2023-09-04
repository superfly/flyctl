// Package lfsc implements the LiteFS Cloud command chain.
package lfsc

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/gql"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/lfsc-go"
)

const (
	tokenExpiry = 1 * time.Minute
)

// New initializes and returns a new apps Command.
func New() *cobra.Command {
	const (
		long = `Commands for managing LiteFS Cloud databases`

		short = "LiteFS Cloud management commands"

		usage = "litefs-cloud <command>"
	)

	cmd := command.New("usage", short, long, nil)
	cmd.Aliases = []string{"litefs-cloud", "lfsc"}

	cmd.AddCommand(
		newClusters(),
		newExport(),
		newImport(),
		newRestore(),
		newStatus(),
	)

	return cmd
}

// urlFlag is a hidden flag for setting the LiteFS Cloud API URL for testing.
func urlFlag() flag.String {
	return flag.String{
		Name:        "url",
		Description: "LiteFS Cloud URL",
		Hidden:      true,
		Default:     lfsc.DefaultURL,
	}
}

func clusterFlag() flag.String {
	return flag.String{
		Name:        "cluster",
		Shorthand:   "c",
		Description: "LiteFS Cloud cluster name",
	}
}

func databaseFlag() flag.String {
	return flag.String{
		Name:        "database",
		Shorthand:   "d",
		Description: "LiteFS Cloud database name",
	}
}

// newLFSCClient returns an lfsc.Client with a temporary auth token.
func newLFSCClient(ctx context.Context, clusterName string) (*lfsc.Client, error) {
	apiClient := client.FromContext(ctx).API()

	// Determine the org via flag or environment variable first.
	// If neither is available, use the local app's org, if available.
	var orgID string
	if slug := flag.GetOrg(ctx); slug != "" {
		org, err := apiClient.GetOrganizationBySlug(ctx, slug)
		if err != nil {
			return nil, fmt.Errorf("failed retrieving organization with slug %s: %w", slug, err)
		}
		orgID = org.ID

	} else {
		appName := appconfig.NameFromContext(ctx)
		if appName == "" {
			return nil, errors.New("no org was provided, and none is available from the environment or fly.toml")
		}

		app, err := apiClient.GetAppCompact(ctx, appName)
		if err != nil {
			return nil, err
		}
		orgID = app.Organization.ID
	}

	// Acquire a temporary auth token to access LiteFS Cloud.
	resp, err := gql.CreateLimitedAccessToken(
		ctx,
		apiClient.GenqClient,
		"flyctl-lfsc",
		orgID,
		"litefs_cloud",
		&gql.LimitedAccessTokenOptions{
			"cluster": clusterName,
		},
		tokenExpiry.String(),
	)
	if err != nil {
		return nil, fmt.Errorf("failed creating litefs-cloud token: %w", err)
	}

	client := lfsc.NewClient()
	client.URL = flag.GetString(ctx, "url")
	client.Token = resp.CreateLimitedAccessToken.LimitedAccessToken.TokenHeader

	return client, nil
}
