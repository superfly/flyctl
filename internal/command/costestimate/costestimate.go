package costestimate

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/internal/buildinfo"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/uiex"
	"github.com/superfly/flyctl/internal/uiexutil"
	"github.com/superfly/flyctl/iostreams"
)

type Input struct {
	Operation     string
	SourceCommand string
	Changes       []uiex.CostEstimateChange
}

func RunForApp(ctx context.Context, appName string, input Input) error {
	client := flyutil.ClientFromContext(ctx)
	if client == nil {
		return fmt.Errorf("can't estimate cost without an API client")
	}

	app, err := client.GetAppCompact(ctx, appName)
	if err != nil {
		return fmt.Errorf("failed fetching app: %w", err)
	}

	return RunForOrg(ctx, app.Organization, input)
}

func RunForOrg(ctx context.Context, org *fly.OrganizationBasic, input Input) error {
	if org == nil || org.Slug == "" {
		return fmt.Errorf("can't estimate cost without an app organization")
	}

	orgSlug, err := ResolveOrgSlug(ctx, org)
	if err != nil {
		return err
	}

	return RunForOrgSlug(ctx, orgSlug, input)
}

func RunForOrgSlug(ctx context.Context, orgSlug string, input Input) error {
	req := uiex.CostEstimateRequest{
		SchemaVersion: 1,
		Operation:     input.Operation,
		Currency:      "USD",
		Changes:       input.Changes,
		Client: &uiex.CostEstimateClient{
			Name:          "flyctl",
			Version:       buildinfo.Version().String(),
			SourceCommand: input.SourceCommand,
		},
	}

	client := uiexutil.ClientFromContext(ctx)
	if client == nil {
		return fmt.Errorf("can't estimate cost without a ui-ex client")
	}

	resp, err := client.CreateCostEstimate(ctx, orgSlug, req)
	if err != nil {
		return err
	}

	var out bytes.Buffer
	if err := json.Indent(&out, resp.Data, "", "  "); err != nil {
		return fmt.Errorf("failed to format cost estimate response: %w", err)
	}
	out.WriteByte('\n')
	_, err = iostreams.FromContext(ctx).Out.Write(out.Bytes())
	return err
}

func ResolveOrgSlug(ctx context.Context, org *fly.OrganizationBasic) (string, error) {
	if org.Slug != "personal" {
		return org.Slug, nil
	}
	if org.RawSlug != "" {
		return org.RawSlug, nil
	}

	client := flyutil.ClientFromContext(ctx)
	if client == nil {
		return "", fmt.Errorf("can't resolve personal organization slug without an API client")
	}

	fullOrg, err := client.GetOrganizationBySlug(ctx, org.Slug)
	if err != nil {
		return "", fmt.Errorf("failed fetching org: %w", err)
	}
	if fullOrg.RawSlug == "" {
		return "", fmt.Errorf("personal organization is missing raw slug")
	}

	return fullOrg.RawSlug, nil
}
