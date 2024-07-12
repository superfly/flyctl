package registry

import (
	"context"
	"fmt"

	"github.com/superfly/macaroon"
	"github.com/superfly/macaroon/flyio"

	"github.com/superfly/flyctl/gql"
	"github.com/superfly/flyctl/internal/flyutil"
)

const (
	appFeatureImages  = "images"
	orgFeatureBuilder = "builder"
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

func attenuateTokens(tokenHeader string, caveats ...macaroon.Caveat) (string, error) {
	toks, err := macaroon.Parse(tokenHeader)
	if err != nil {
		return "", fmt.Errorf("failed to parse token: %w", err)
	}

	perms, _, _, retToks, err := macaroon.FindPermissionAndDischargeTokens(toks, flyio.LocationPermission)
	switch {
	case err != nil:
		return "", fmt.Errorf("failed to find permission tokens: %w", err)
	case len(perms) == 0:
		return "", fmt.Errorf("no permission tokens found")
	}

	for _, perm := range perms {
		if err := perm.Add(caveats...); err != nil {
			return "", fmt.Errorf("failed to attenuate token: %w", err)
		}

		tok, err := perm.Encode()
		if err != nil {
			return "", fmt.Errorf("failed to encode token: %w", err)
		}

		retToks = append(retToks, tok)
	}

	return macaroon.ToAuthorizationHeader(retToks...), nil
}
