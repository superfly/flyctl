package config

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/superfly/fly-go/tokens"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/logger"
	"github.com/superfly/macaroon"
	"github.com/superfly/macaroon/flyio"
	"github.com/superfly/macaroon/resset"
	"github.com/superfly/macaroon/tp"
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

// TestRefreshDischargeTokensPartialSuccess covers a set of tickets where one
// discharges on its own and another wants to send the user to their browser.
// The parallel pass collects the first and fails on the second; the retry with
// a callback then fails too. The discharge from the first pass is still a real
// update and has to be reported, or our callers never write it to the config
// file and every subsequent command fetches it again.
func TestRefreshDischargeTokensPartialSuccess(t *testing.T) {
	ctx := logger.NewContext(context.Background(), logger.New(os.Stdout, logger.Debug, true))

	var (
		m     sync.Mutex
		inits int
	)

	thirdParty := fakeThirdParty(t, func(w http.ResponseWriter, r *http.Request, tp3 *tp.TP) {
		m.Lock()
		inits++
		first := inits == 1
		m.Unlock()

		// the first ticket to arrive is discharged outright; anything after it
		// needs the user.
		if first {
			tp3.RespondDischarge(w, r)

			return
		}

		tp3.RespondUserInteractive(w, r)
	})

	var callbacks int
	uucb := func(context.Context, string) error {
		callbacks++

		return errors.New("no browser here")
	}

	updated, err := refreshDischargeTokens(ctx, fakeThirdPartyTokens(t, thirdParty, 1, 2), uucb, time.Minute)

	require.Error(t, err)
	require.Equal(t, 1, callbacks, "the interactive retry should have run")
	require.True(t, updated, "the discharge from the parallel pass was dropped")
}

// TestRefreshDischargeTokensTimeouts checks that each discharge pass is bounded
// by the budget sized for what it actually waits on: a server round trip for
// the pass that cannot open a browser, a person for the pass that can.
func TestRefreshDischargeTokensTimeouts(t *testing.T) {
	ctx := logger.NewContext(context.Background(), logger.New(os.Stdout, logger.Debug, true))

	const (
		nonInteractive = 250 * time.Millisecond
		interactive    = 2 * time.Second
	)

	t.Run("a pass with no user in it stops at the short budget", func(t *testing.T) {
		// polls forever without ever discharging, and never asks for the user,
		// so there is no interactive retry to fall through to.
		thirdParty := fakeThirdParty(t, func(w http.ResponseWriter, r *http.Request, tp3 *tp.TP) {
			tp3.RespondPoll(w, r)
		})

		uucb := func(context.Context, string) error {
			t.Error("should not have been asked to open a browser")

			return nil
		}

		start := time.Now()
		_, err := doRefreshDischargeTokens(ctx, fakeThirdPartyTokens(t, thirdParty, 1), uucb, time.Minute, nonInteractive, interactive)
		took := time.Since(start)

		require.ErrorIs(t, err, context.DeadlineExceeded)
		require.Less(t, took, interactive, "the pass ran on the interactive budget")
	})

	t.Run("the interactive retry gets the long budget", func(t *testing.T) {
		// asks for the user every time, so the first pass fails immediately and
		// the retry polls for a discharge that never comes.
		thirdParty := fakeThirdParty(t, func(w http.ResponseWriter, r *http.Request, tp3 *tp.TP) {
			tp3.RespondUserInteractive(w, r)
		})

		uucb := func(context.Context, string) error { return nil }

		start := time.Now()
		_, err := doRefreshDischargeTokens(ctx, fakeThirdPartyTokens(t, thirdParty, 1), uucb, time.Minute, nonInteractive, interactive)
		took := time.Since(start)

		require.ErrorIs(t, err, context.DeadlineExceeded)
		require.Greater(t, took, interactive-nonInteractive, "the retry ran on the non-interactive budget")
	})
}

// fakeThirdParty stands up a third party whose discharge requests are answered
// by respond.
func fakeThirdParty(tb testing.TB, respond func(w http.ResponseWriter, r *http.Request, tp3 *tp.TP)) *tp.TP {
	tb.Helper()

	var thirdParty *tp.TP

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch path := r.URL.EscapedPath(); {
		case path == tp.InitPath:
			thirdParty.InitRequestMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				respond(w, r, thirdParty)
			})).ServeHTTP(w, r)
		case strings.HasPrefix(path, tp.PollPathPrefix):
			thirdParty.HandlePollRequest(w, r)
		default:
			tb.Errorf("unexpected request to %s", path)
		}
	}))
	tb.Cleanup(server.Close)

	store, err := tp.NewMemoryStore(tp.PrefixMunger("/user/"), 100)
	require.NoError(tb, err)

	thirdParty = &tp.TP{
		Location: server.URL,
		Key:      macaroon.NewEncryptionKey(),
		Store:    store,
	}

	return thirdParty
}

// fakeThirdPartyTokens returns permission tokens for the given orgs, each with
// an undischarged third party caveat for thirdParty.
func fakeThirdPartyTokens(tb testing.TB, thirdParty *tp.TP, oids ...uint64) *tokens.Tokens {
	tb.Helper()

	toks := make([][]byte, 0, len(oids))
	for _, oid := range oids {
		perm := fakePermissionToken(tb, &flyio.Organization{ID: oid, Mask: resset.ActionAll})
		require.NoError(tb, perm.Add3P(thirdParty.Key, thirdParty.Location))

		tok, err := perm.Encode()
		require.NoError(tb, err)
		toks = append(toks, tok)
	}

	return tokens.Parse(macaroon.ToAuthorizationHeader(toks...))
}
