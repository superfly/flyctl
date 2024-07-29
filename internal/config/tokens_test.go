package config

import (
	"context"
	"errors"
	"os"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/superfly/fly-go/tokens"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/logger"
	"github.com/superfly/macaroon"
	"github.com/superfly/macaroon/flyio"
	"github.com/superfly/macaroon/resset"
)

func TestFetchOrgTokens(t *testing.T) {
	ctx := logger.NewContext(context.Background(), logger.New(os.Stdout, logger.Debug, true))

	// no tokens
	created, err := doFetchOrgTokens(ctx, &tokens.Tokens{}, nil, nil)
	require.False(t, created)
	require.NoError(t, err)

	// no macaroons
	created, err = doFetchOrgTokens(ctx, tokens.Parse("fo1_hi"), nil, nil)
	require.False(t, created)
	require.NoError(t, err)

	// no user token
	created, err = doFetchOrgTokens(ctx, tokens.Parse("fm2_hi"), nil, nil)
	require.False(t, created)
	require.NoError(t, err)

	// basic case
	toks := fakeTokens(t, "fo1_hi", 1)
	fetchOrgs := fakeOrgFetcher(map[uint64]string{1: "org1", 2: "org2"}, nil)
	mintToken := fakeOrgTokenMinter(t, "org2", 2)
	created, err = doFetchOrgTokens(ctx, toks, fetchOrgs, mintToken)
	require.True(t, created)
	require.NoError(t, err)
	assertTokenOrgs(t, toks, 1, 2)

	// fetchOrgs error
	toks = fakeTokens(t, "fo1_hi", 1)
	foErr := errors.New("my error")
	fetchOrgs = fakeOrgFetcher(nil, foErr)
	created, err = doFetchOrgTokens(ctx, toks, fetchOrgs, nil)
	require.False(t, created)
	require.ErrorIs(t, err, foErr)

	// partial success
	toks = fakeTokens(t, "fo1_hi", 1)
	fetchOrgs = fakeOrgFetcher(map[uint64]string{1: "org1", 2: "org2", 3: "org3"}, nil)
	fotErr := errors.New("my error")
	mintToken = fakeTokenMinter(
		fakeTokenHeader(t, "", 2),
		fotErr,
	)
	created, err = doFetchOrgTokens(ctx, toks, fetchOrgs, mintToken)
	require.True(t, created)
	require.ErrorIs(t, err, fotErr)
	assertTokenOrgs(t, toks, 1, 2)

	// prune tokens for orgs that user isn't member of
	toks = fakeTokens(t, "fo1_hi", 1, 2)
	fetchOrgs = fakeOrgFetcher(map[uint64]string{1: "org1"}, nil)
	created, err = doFetchOrgTokens(ctx, toks, fetchOrgs, nil)
	require.True(t, created)
	require.NoError(t, err)
	assertTokenOrgs(t, toks, 1)
}

func fakeOrgFetcher(orgs map[uint64]string, err error) orgFetcher {
	return func(context.Context, flyutil.Client) (map[uint64]string, error) { return orgs, err }
}

func fakeOrgTokenMinter(tb testing.TB, expectedGraphID string, oid uint64) tokenMinter {
	tb.Helper()
	return func(_ context.Context, _ flyutil.Client, graphID string) (string, error) {
		require.Equal(tb, expectedGraphID, graphID)
		return fakeTokenHeader(tb, "", oid), nil
	}
}

func fakeTokenMinter(hdrsOrErrors ...any) tokenMinter {
	return func(context.Context, flyutil.Client, string) (string, error) {
		if len(hdrsOrErrors) == 0 {
			panic("unexpected call to fakeTokenMinter")
		}

		hdrOrErr := hdrsOrErrors[0]
		hdrsOrErrors = hdrsOrErrors[1:]

		switch hoe := hdrOrErr.(type) {
		case error:
			return "", hoe
		case string:
			return hoe, nil
		default:
			panic("unexpected type")
		}
	}
}

var (
	permKID = []byte("hello")
	permK   = macaroon.NewSigningKey()
	authK   = macaroon.NewEncryptionKey()
)

func fakeTokens(tb testing.TB, userToken string, oids ...uint64) *tokens.Tokens {
	tb.Helper()

	return tokens.Parse(fakeTokenHeader(tb, userToken, oids...))
}

func fakeTokenHeader(tb testing.TB, userToken string, oids ...uint64) string {
	tb.Helper()

	macs := fakeMacaroons(tb, oids...)
	toks := make([][]byte, 0, len(macs))
	for _, m := range macs {
		tok, err := m.Encode()
		require.NoError(tb, err)
		toks = append(toks, tok)
	}

	hdr := macaroon.ToAuthorizationHeader(toks...)

	if userToken != "" {
		if len(toks) > 0 {
			hdr += "," + userToken
		} else {
			hdr += userToken
		}
	}

	return hdr
}

func fakeMacaroons(tb testing.TB, oids ...uint64) []*macaroon.Macaroon {
	tb.Helper()

	toks := make([]*macaroon.Macaroon, 0, len(oids)*2)
	for _, oid := range oids {
		perm := fakePermissionToken(tb, &flyio.Organization{ID: oid, Mask: resset.ActionAll})
		auth := fakeAuthToken(tb, perm)
		toks = append(toks, perm, auth)
	}

	return toks
}

func fakePermissionToken(tb testing.TB, cavs ...macaroon.Caveat) *macaroon.Macaroon {
	tb.Helper()

	perm, err := macaroon.New(permKID, flyio.LocationPermission, permK)
	require.NoError(tb, err)
	require.NoError(tb, perm.Add(cavs...))
	return perm
}

func fakeAuthToken(tb testing.TB, perm *macaroon.Macaroon) *macaroon.Macaroon {
	tb.Helper()

	require.NoError(tb, perm.Add3P(authK, flyio.LocationAuthentication))
	ticket, err := perm.ThirdPartyTicket(flyio.LocationAuthentication)
	require.NoError(tb, err)
	_, auth, err := macaroon.DischargeTicket(authK, flyio.LocationAuthentication, ticket)
	require.NoError(tb, err)
	return auth
}

func assertTokenOrgs(tb testing.TB, toks *tokens.Tokens, expectedOIDs ...uint64) {
	tb.Helper()

	actualOIDs := make([]uint64, 0, len(expectedOIDs))
	for _, mt := range toks.GetMacaroonTokens() {
		mtoks, err := macaroon.Parse(mt)
		require.NoError(tb, err)
		require.Equal(tb, 1, len(mtoks))
		macs, _, _, _, err := macaroon.FindPermissionAndDischargeTokens(mtoks, flyio.LocationPermission)
		require.NoError(tb, err)
		if len(macs) != 1 {
			continue
		}
		oid, err := flyio.OrganizationScope(&macs[0].UnsafeCaveats)
		require.NoError(tb, err)
		actualOIDs = append(actualOIDs, oid)
	}

	slices.Sort(expectedOIDs)
	slices.Sort(actualOIDs)
	require.Equal(tb, expectedOIDs, actualOIDs)
}
