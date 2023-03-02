package appsv2

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/gql"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
)

func newDefaultOn() *cobra.Command {
	cmd := command.New(
		`default-on <org-slug>`,
		`Default to using apps v2 for org`,
		`Configure this org to use apps v2 by default for new apps`,
		runDefaultOn,
		command.RequireSession,
	)
	cmd.Args = cobra.ExactArgs(1)
	return cmd
}

func newDefaultOff() *cobra.Command {
	cmd := command.New(
		`default-off <org-slug>`,
		`Default to _not_ using apps v2 for org`,
		`Configure this org to _not_ use apps v2 by default for new apps`,
		runDefaultOff,
		command.RequireSession,
	)
	cmd.Args = cobra.ExactArgs(1)
	return cmd
}

func runDefaultOn(ctx context.Context) error {
	var (
		apiClient = client.FromContext(ctx).API()
		orgSlug   = flag.FirstArg(ctx)
	)
	err := setAppsV2DefaultOnForOrg(ctx, orgSlug, true, apiClient)
	if err != nil {
		return fmt.Errorf("failed to set apps v2 setting due to error: %w", err)
	}
	return runShow(ctx)
}

func runDefaultOff(ctx context.Context) error {
	var (
		apiClient = client.FromContext(ctx).API()
		orgSlug   = flag.FirstArg(ctx)
	)
	err := setAppsV2DefaultOnForOrg(ctx, orgSlug, false, apiClient)
	if err != nil {
		return fmt.Errorf("failed to set apps v2 setting due to error: %w", err)
	}
	return runShow(ctx)
}

func setAppsV2DefaultOnForOrg(ctx context.Context, orgSlug string, appsV2DefaultOn bool, apiClient *api.Client) error {
	_ = `# @genqlient
	mutation SetOrgSettings($input:SetAppsv2DefaultOnInput!) {
		setAppsV2DefaultOn(input:$input) {
			organization {
				settings
			}
		}
	}
	`
	_, err := gql.SetOrgSettings(ctx, apiClient.GenqClient, gql.SetAppsv2DefaultOnInput{
		OrganizationSlug: orgSlug,
		DefaultOn:        appsV2DefaultOn,
	})
	if err != nil {
		return err
	}
	return nil
}
