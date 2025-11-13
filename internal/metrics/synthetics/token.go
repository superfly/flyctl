package synthetics

import (
	"context"
	"fmt"

	"github.com/superfly/flyctl/gql"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/uiex"
	"github.com/superfly/flyctl/internal/uiexutil"
	"github.com/superfly/macaroon"
	"github.com/superfly/macaroon/flyio"
)

func generateSyntheticsToken(ctx context.Context) (token string, err error) {
	client := uiexutil.ClientFromContext(ctx)
	var orgs []uiex.Organization
	if orgs, err = client.ListOrganizations(ctx, false); err != nil {
		return
	}

	var orgsTokens [][]byte

	for _, org := range orgs {
		var orgToken [][]byte

		orgToken, err = getOrgReadOnlyToken(ctx, org)
		if err != nil {
			return
		}

		orgsTokens = append(orgsTokens, orgToken...)
	}

	authHeader := macaroon.ToAuthorizationHeader(orgsTokens...)

	return authHeader, nil
}

func GetSyntheticsToken(ctx context.Context) (token string, err error) {
	// Prevent synthetics panics from bubbling up to the user.
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic: %v", r)
		}
	}()

	token, err = generateSyntheticsToken(ctx)
	if err != nil {
		return "", err
	}
	return token, nil
}

func getOrgReadOnlyToken(ctx context.Context, org uiex.Organization) ([][]byte, error) {
	var (
		token     string
		apiClient = flyutil.ClientFromContext(ctx)
		expiry    = "1h"
		profile   = "flynthetics_read"
		name      = "Flynthetics read-only token"
		perm      []byte
		diss      [][]byte
	)
	resp, err := gql.CreateLimitedAccessToken(
		ctx,
		apiClient.GenqClient(),
		name,
		org.ID,
		profile,
		&gql.LimitedAccessTokenOptions{},
		expiry,
	)
	if err != nil {
		return nil, err
	}

	token = resp.CreateLimitedAccessToken.LimitedAccessToken.TokenHeader

	perm, diss, err = macaroon.ParsePermissionAndDischargeTokens(token, flyio.LocationPermission)
	if err != nil {
		return nil, err
	}

	tokens := append([][]byte{perm}, diss...)

	return tokens, nil
}
