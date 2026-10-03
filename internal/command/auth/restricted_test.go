package auth

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/macaroon"
	"github.com/superfly/macaroon/flyio"
	"github.com/superfly/macaroon/resset"
)

func TestParseMask(t *testing.T) {
	mask, err := parseMask("rwcC")
	require.NoError(t, err)
	require.Equal(t,
		resset.ActionRead|resset.ActionWrite|resset.ActionCreate|
			resset.ActionControl,
		mask,
	)

	_, err = parseMask("rwx")
	require.Error(t, err)
}

func TestOrgsBySlug(t *testing.T) {
	orgs := []fly.Organization{
		{
			ID:      "a",
			Slug:    "personal",
			RawSlug: "jane-doe",
		},
		{
			ID:   "b",
			Slug: "acme",
		},
	}

	selected, err := orgsBySlug(orgs, []string{"acme", "jane-doe"})
	require.NoError(t, err)
	require.Equal(t, "b", selected[0].ID)
	require.Equal(t, "a", selected[1].ID)

	_, err = orgsBySlug(orgs, []string{"nope"})
	require.Error(t, err)
}

func TestMintRestrictedTokenDropsDelete(t *testing.T) {
	const orgID = uint64(1234)

	key := macaroon.NewSigningKey()
	mac, err := macaroon.New([]byte("kid"), flyio.LocationPermission, key)
	require.NoError(t, err)
	require.NoError(t, mac.Add(&flyio.Organization{
		ID:   orgID,
		Mask: resset.ActionAll,
	}))
	tok, err := mac.Encode()
	require.NoError(t, err)

	var gotExpiry string
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			var body struct {
				Variables map[string]any `json:"variables"`
			}
			require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
			gotExpiry, _ = body.Variables["expiry"].(string)

			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w,
				`{"data":{"createLimitedAccessToken":`+
					`{"limitedAccessToken":{"tokenHeader":%q}}}}`,
				macaroon.ToAuthorizationHeader(tok),
			)
		},
	))
	defer server.Close()
	fly.SetBaseURL(server.URL)

	mask, err := parseMask("rwcC")
	require.NoError(t, err)

	client := flyutil.NewClientFromOptions(t.Context(), fly.ClientOptions{
		AccessToken: "user-token",
	})
	toks, err := mintRestrictedToken(t.Context(),
		client,
		fly.Organization{
			ID:   "org-graph-id",
			Slug: "acme",
		},
		mask,
		time.Hour,
	)
	require.NoError(t, err)
	require.Len(t, toks, 1)
	require.Equal(t, "1h0m0s", gotExpiry)

	restricted, err := macaroon.Decode(toks[0])
	require.NoError(t, err)
	cavs, err := restricted.Verify(key, nil, nil)
	require.NoError(t, err)

	id := orgID
	require.NoError(t, cavs.Validate(&flyio.Access{
		OrgID:  &id,
		Action: resset.ActionWrite,
	}))
	require.Error(t, cavs.Validate(&flyio.Access{
		OrgID:  &id,
		Action: resset.ActionDelete,
	}))
}
