package scan

import (
	"context"
	"fmt"
	"net/http"

	fly "github.com/superfly/fly-go"
	"github.com/superfly/macaroon"
	"github.com/superfly/macaroon/flyio"
	"github.com/superfly/macaroon/resset"

	"github.com/superfly/flyctl/gql"
	"github.com/superfly/flyctl/internal/flyutil"
)

const (
	appFeatureImages  = "images"
	orgFeatureBuilder = "builder"
)

func addAuth(ctx context.Context, apiClient flyutil.Client, orgId, appId string, req *http.Request) error {
	resp, err := makeToken(ctx, apiClient, "ScantronToken", orgId, "5m", "deploy", &gql.LimitedAccessTokenOptions{
		"app_id": appId,
	})
	if err != nil {
		return err
	}

	token := resp.CreateLimitedAccessToken.LimitedAccessToken.TokenHeader
	token, err = attenuateTokens(token,
		&resset.IfPresent{
			Ifs: macaroon.NewCaveatSet(
				&flyio.FeatureSet{Features: resset.New[string](resset.ActionRead, orgFeatureBuilder)},
				&flyio.AppFeatureSet{Features: resset.New[string](resset.ActionRead, appFeatureImages)},
			),
			Else: resset.ActionNone,
		},
	)
	if err != nil {
		return err
	}

	req.Header.Set("Authorization", fly.AuthorizationHeader(token))
	return nil
}

func makeToken(ctx context.Context, apiClient flyutil.Client, name, orgID, expiry, profile string, options *gql.LimitedAccessTokenOptions) (*gql.CreateLimitedAccessTokenResponse, error) {
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
