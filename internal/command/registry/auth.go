package registry

import (
	"context"
	"fmt"

	"github.com/superfly/flyctl/gql"
	"github.com/superfly/flyctl/internal/flyutil"
)

func makeToken(ctx context.Context, name, orgID, expiry, profile string, options *gql.LimitedAccessTokenOptions) (*gql.CreateLimitedAccessTokenResponse, error) {
	apiClient := flyutil.ClientFromContext(ctx)
	resp, err := gql.CreateLimitedAccessToken(
		ctx,
		apiClient.GenqClient(),
		name,
		orgID,
		profile,
		options,
		expiry,
	)
	if err != nil {
		return nil, fmt.Errorf("failed creating token: %w", err)
	}
	return resp, nil
}
