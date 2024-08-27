package synthetics

import (
	"context"
	"errors"
	"fmt"

	"github.com/superfly/fly-go"
	"github.com/superfly/flyctl/gql"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/state"
	"github.com/superfly/flyctl/terminal"
	"github.com/superfly/macaroon"
	"github.com/superfly/macaroon/flyio"
	"github.com/superfly/macaroon/resset"
)

func generateSyntheticsToken(ctx context.Context) (token string, err error) {
	client := flyutil.ClientFromContext(ctx)
	var orgs []fly.Organization
	if orgs, err = client.GetOrganizations(ctx); err != nil {
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

	cfg := config.FromContext(ctx)
	if cfg.SyntheticsToken != "" {
		terminal.Debugf("Config has synthetics token\n")
		return cfg.SyntheticsToken, nil
	}

	if cfg.SyntheticsToken == "" && cfg.Tokens.GraphQL() != "" {
		terminal.Debugf("Generating synthetics token\n")
		token, err := generateSyntheticsToken(ctx)
		if err != nil {
			return "", err
		}
		if err = persistSyntheticsToken(ctx, token); err != nil {
			return "", err
		}
		cfg.SyntheticsToken = token
		return token, nil
	}
	return "", errors.New("failed to get synthetics token")
}

func getOrgReadOnlyToken(ctx context.Context, org fly.Organization) ([][]byte, error) {
	var (
		token     string
		apiClient = flyutil.ClientFromContext(ctx)
		expiry    = "168h" // 1w
		profile   = "deploy_organization"
		name      = "Read-only org token"
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

	mac, err := macaroon.Decode(perm)
	if err != nil {
		return nil, err
	}

	// attenuate to read-only
	var orgID *uint64
	for _, cav := range macaroon.GetCaveats[*flyio.Organization](&mac.UnsafeCaveats) {
		if orgID != nil {
			return nil, errors.New("multiple org caveats")
		}
		orgID = &cav.ID
	}
	if orgID == nil {
		return nil, errors.New("no org caveats")
	}
	if err := mac.Add(&flyio.Organization{ID: *orgID, Mask: resset.ActionRead}); err != nil {
		return nil, err
	}

	if perm, err = mac.Encode(); err != nil {
		return nil, err
	}

	tokens := append([][]byte{perm}, diss...)

	return tokens, nil
}

func persistSyntheticsToken(ctx context.Context, token string) error {
	path := state.ConfigFile(ctx)

	if err := config.SetSyntheticsToken(path, token); err != nil {
		return fmt.Errorf("failed persisting %s in %s: %w",
			config.SyntheticsTokenFileKey, path, err)
	}
	return nil
}
