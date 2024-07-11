package imgsrc

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/superfly/fly-go"
	"github.com/superfly/fly-go/tokens"
	"github.com/superfly/flyctl/gql"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/macaroon"
	"github.com/superfly/macaroon/flyio"
	"github.com/superfly/macaroon/resset"
)

const (
	buildTokenName   = "Temporary flyctl build token"
	buildTokenExpiry = 1 * time.Hour
)

func getBuildToken(ctx context.Context, app *fly.AppCompact) (string, error) {
	tokens := config.Tokens(ctx)

	orgID, err := strconv.ParseUint(app.Organization.InternalNumericID, 10, 64)
	if err != nil {
		return "", fmt.Errorf("failed to parse organization ID: %w", err)
	}

	var token string
	if len(tokens.GetUserTokens()) > 0 {
		token, err = getBuildTokenFromUser(ctx, orgID, app)
	} else {
		token, err = getBuildTokenFromMacaroons(tokens.GetMacaroonTokens(), orgID, app.InternalNumericID)
	}

	if err != nil {
		return "", fmt.Errorf("failed to create build token: %w", err)
	}

	return token, nil
}

func RevokeBuildTokens(ctx context.Context, app *fly.AppCompact) error {
	cfg := config.FromContext(ctx)
	cachedToken, ok := cfg.CachedBuildTokens[app.InternalNumericID]
	if !ok {
		return nil
	}
	delete(cfg.CachedBuildTokens, app.InternalNumericID)

	apiClient := flyutil.ClientFromContext(ctx)
	return apiClient.RevokeLimitedAccessToken(ctx, cachedToken.ID)
}

func addBuildTokenCaveats(m *macaroon.Macaroon, appID uint64, includeExpiry bool) {
	appFeatureImages := "images"
	orgFeatureBuilder := flyio.FeatureRemoteBuilders

	m.Add(&resset.IfPresent{
		Ifs: macaroon.NewCaveatSet(
			// Restrict to current app
			&flyio.Apps{
				Apps: resset.ResourceSet[uint64]{
					appID: resset.ActionAll,
				},
			},
		),
		Else: resset.ActionAll,
	})
	m.Add(&resset.IfPresent{
		Ifs: macaroon.NewCaveatSet(
			// Write image for current app
			&flyio.AppFeatureSet{
				Features: resset.ResourceSet[string]{
					appFeatureImages: resset.ActionRead | resset.ActionWrite,
				},
			},
			// Control builder and read images for other apps
			&flyio.FeatureSet{
				Features: resset.ResourceSet[string]{
					orgFeatureBuilder: resset.ActionRead | resset.ActionControl,
				},
			},
		),
		// Otherwise, read-only access to org and app resources
		Else: resset.ActionRead,
	})

	if includeExpiry {
		m.Add(&macaroon.ValidityWindow{
			NotBefore: time.Now().Add(-(30 * time.Second)).Unix(),
			NotAfter:  time.Now().Add(buildTokenExpiry).Unix(),
		})
	}
}

func encodeMacaroons(toks [][]byte) string {
	return tokens.StripAuthorizationScheme(macaroon.ToAuthorizationHeader(toks...))
}

func getBuildTokenFromUser(ctx context.Context, orgID uint64, app *fly.AppCompact) (string, error) {
	appID := app.InternalNumericID
	cfg := config.FromContext(ctx)

	// If we have an unexpired token for this organization, return it
	if cachedToken, ok := cfg.CachedBuildTokens[appID]; ok {
		expired := time.Now().Add(time.Minute).After(cachedToken.Expiration)
		if !expired {
			return cachedToken.Token, nil
		}
	}

	// Otherwise, we need to create a token for this organization
	apiClient := flyutil.ClientFromContext(ctx)
	resp, err := gql.CreateLimitedAccessToken(
		ctx,
		apiClient.GenqClient(),
		buildTokenName,
		app.Organization.ID,
		"deploy",
		&gql.LimitedAccessTokenOptions{
			"app_id": app.ID,
		},
		buildTokenExpiry.String(),
	)
	if err != nil {
		return "", err
	}

	toks, err := macaroon.Parse(resp.CreateLimitedAccessToken.LimitedAccessToken.TokenHeader)
	if err != nil {
		return "", err
	}

	perms, _, disMacs, disToks, err := macaroon.FindPermissionAndDischargeTokens(toks, flyio.LocationPermission)
	if err != nil {
		return "", err
	}
	if len(perms) != 1 {
		return "", errors.New("expected exactly one permission token")
	}

	// Mask access, but skip expiry because we already specified it
	m := perms[0]
	addBuildTokenCaveats(m, appID, false)

	perm, err := m.Encode()
	if err != nil {
		return "", err
	}

	token := encodeMacaroons(append([][]byte{perm}, disToks...))

	// Find the earliest time any of the tokens expire
	expiration := m.Expiration()
	for _, dis := range disMacs {
		if e := dis.Expiration(); e.Before(expiration) {
			expiration = e
		}
	}

	// Cache token and expiration time
	if cfg.CachedBuildTokens == nil {
		cfg.CachedBuildTokens = make(map[uint64]config.CachedBuildToken)
	}
	cfg.CachedBuildTokens[appID] = config.CachedBuildToken{
		ID:         resp.CreateLimitedAccessToken.LimitedAccessToken.Id,
		Token:      token,
		Expiration: expiration,
	}

	return token, nil
}

func getBuildTokenFromMacaroons(macaroons []string, orgID, appID uint64) (string, error) {
	var raws [][]byte
	for _, m := range macaroons {
		toks, err := macaroon.Parse(m)
		if err != nil {
			return "", err
		}
		raws = append(raws, toks...)
	}

	perms, _, disMacs, disToks, err := macaroon.FindPermissionAndDischargeTokens(raws, flyio.LocationPermission)
	if err != nil {
		return "", err
	}

	dischargeByTicket := make(map[string][]byte)
	for i, m := range disMacs {
		dischargeByTicket[hex.EncodeToString(m.Nonce.KID)] = disToks[i]
	}

	var toks [][]byte
	for _, m := range perms {
		// Skip tokens for other organizations
		orgScope, err := flyio.OrganizationScope(&m.UnsafeCaveats)
		if err != nil || orgScope != orgID {
			continue
		}

		// Mask access and add expiry
		addBuildTokenCaveats(m, appID, true)

		tok, err := m.Encode()
		if err != nil {
			return "", fmt.Errorf("unable to encode macaroon: %w", err)
		}

		toks = append(toks, tok)

		// Append all the relevant discharge tokens
		for _, cav := range macaroon.GetCaveats[*macaroon.Caveat3P](&m.UnsafeCaveats) {
			if disTok, ok := dischargeByTicket[hex.EncodeToString(cav.Ticket)]; ok {
				toks = append(toks, disTok)
			}
		}
	}

	if len(toks) == 0 {
		return "", errors.New("no valid tokens for organization")
	}

	return encodeMacaroons(toks), nil
}
