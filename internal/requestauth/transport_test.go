package requestauth_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/fly-go/tokens"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/requestauth"
	"github.com/superfly/flyctl/internal/uiex"
	mpgv1 "github.com/superfly/flyctl/internal/uiex/mpg/v1"
	"github.com/superfly/macaroon"
	"github.com/superfly/macaroon/flyio"
	"github.com/superfly/macaroon/resset"
)

type roundTrip func(*http.Request) (*http.Response, error)

func (f roundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func session(t *testing.T) (*tokens.Tokens, []string) {
	all, roots := []string{"fo1_local-test"}, []string{}
	for id := uint64(1); id <= 150; id++ {
		m, err := macaroon.New([]byte("local-test"), flyio.LocationPermission, macaroon.NewSigningKey())
		require.NoError(t, err)
		require.NoError(t, m.Add(&flyio.Organization{ID: id, Mask: resset.ActionAll}))
		key := macaroon.NewEncryptionKey()
		require.NoError(t, m.Add3P(key, flyio.LocationAuthentication))
		ticket, err := m.ThirdPartyTicket(flyio.LocationAuthentication)
		require.NoError(t, err)
		_, d, err := macaroon.DischargeTicket(key, flyio.LocationAuthentication, ticket)
		require.NoError(t, err)
		root, err := m.String()
		require.NoError(t, err)
		discharge, err := d.String()
		require.NoError(t, err)
		roots = append(roots, root+","+discharge)
		all = append(all, root, discharge)
	}
	tok := tokens.Parse(strings.Join(all, ","))
	require.Greater(t, len(tok.FlapsHeader()), 49152)
	return tok, roots
}

func TestSelection(t *testing.T) {
	tok, roots := session(t)
	// Permission roots without an org restriction must survive selection too.
	unscoped, err := macaroon.New([]byte("local-test"), flyio.LocationPermission, macaroon.NewSigningKey())
	require.NoError(t, err)
	unscopedToken, err := unscoped.String()
	require.NoError(t, err)
	tok = tokens.Parse(tok.All() + "," + unscopedToken)
	parent := requestauth.WithOrganizations(context.Background(), 1)
	for _, tc := range []struct {
		name      string
		ids       []uint64
		unchanged bool
	}{
		{"single", []uint64{2}, false}, {"cross org", []uint64{2, 3}, false},
		{"global", nil, true}, {"unknown", []uint64{0}, true},
		{"missing", []uint64{151}, true}, {"incomplete", []uint64{2, 151}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := requestauth.WithOrganizations(parent, tc.ids...)
			req, err := http.NewRequestWithContext(ctx, "POST", "http://local/apps", strings.NewReader("exact body"))
			require.NoError(t, err)
			header := tok.GraphQLHeader()
			req.Header.Set("Authorization", header)
			calls := 0
			failure := errors.New("after write")
			tr := requestauth.Transport(roundTrip(func(r *http.Request) (*http.Response, error) {
				calls++
				body, err := io.ReadAll(r.Body)
				require.NoError(t, err)
				require.Equal(t, "exact body", string(body))
				if tc.unchanged {
					require.Equal(t, header, r.Header.Get("Authorization"))
				} else {
					expected := []string{"fo1_local-test", unscopedToken}
					for _, id := range tc.ids {
						expected = append(expected, strings.Split(roots[id-1], ",")...)
					}
					require.ElementsMatch(t, expected, strings.Split(strings.TrimPrefix(r.Header.Get("Authorization"), "FlyV1 "), ","))
				}
				return nil, failure
			}))
			_, err = tr.RoundTrip(req)
			require.ErrorIs(t, err, failure)
			require.Equal(t, 1, calls)
			require.Equal(t, header, req.Header.Get("Authorization"))
		})
	}
	for _, header := range []string{"Bearer fo1_only", "FlyV1 broken", "no-scheme"} {
		req, _ := http.NewRequestWithContext(parent, "GET", "http://local", nil)
		req.Header.Set("Authorization", header)
		_, _ = requestauth.Transport(roundTrip(func(r *http.Request) (*http.Response, error) {
			require.Equal(t, header, r.Header.Get("Authorization"))
			return nil, errors.New("stop")
		})).RoundTrip(req)
	}
}

// Real clients must select before the first send, independently of its status.
func TestClientsSelectCurrentCredentials(t *testing.T) {
	tok, roots := session(t)
	ctx := config.NewContext(context.Background(), &config.Config{Tokens: tok})
	expected := roots[1]
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		header := r.Header.Get("Authorization")
		require.Less(t, len(header), 4096)
		for _, part := range strings.Split(expected, ",") {
			require.Contains(t, header, part)
		}
		require.NotContains(t, header, strings.Split(roots[0], ",")[0])
		if strings.HasPrefix(r.URL.Path, "/api/") {
			require.Contains(t, header, "fo1_local-test")
		} else {
			require.NotContains(t, header, "fo1_local-test")
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/apps/demo":
			io.WriteString(w, `{"name":"demo"}`)
		case "/v1/postgres":
			io.WriteString(w, `{"data":[]}`)
		default:
			io.WriteString(w, `{"data":[],"releases":[]}`)
		}
	}))
	defer server.Close()
	endpoint, err := url.Parse(server.URL)
	require.NoError(t, err)
	t.Setenv("FLY_FLAPS_BASE_URL", server.URL)
	f, err := flapsutil.NewClientWithOptions(ctx, flaps.NewClientOpts{Tokens: tok})
	require.NoError(t, err)
	u, err := uiex.NewWithOptions(ctx, uiex.NewClientOpts{BaseURL: endpoint, Tokens: tok})
	require.NoError(t, err)
	legacy, err := mpgv1.NewClientWithOptions(ctx, uiex.NewClientOpts{BaseURL: endpoint, Tokens: tok})
	require.NoError(t, err)
	scoped := requestauth.WithOrganizations(ctx, 2)
	for n := 0; n < 2; n++ {
		original := tok.All()
		_, err = f.GetApp(scoped, "demo")
		require.NoError(t, err)
		_, err = f.ListManagedPostgresClusters(scoped, flaps.ListManagedPostgresClustersRequest{OrgSlug: "team"})
		require.NoError(t, err)
		_, err = u.ListReleases(scoped, "demo", 1)
		require.NoError(t, err)
		_, err = legacy.ListManagedClusters(scoped, "team", false)
		require.NoError(t, err)
		require.Equal(t, original, tok.All())
		fresh, next := session(t)
		tok.Replace(fresh)
		roots = next
		expected = roots[1]
	}
	require.Equal(t, 8, calls)
}
