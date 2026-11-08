package server

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/fly-go/tokens"
)

func TestFetchOrg(t *testing.T) {
	for _, tc := range []struct {
		name, slug, canonical, response, wantError string
		serverTokens                               bool
	}{
		{
			name: "restricted session token", slug: "team", canonical: "team",
			response: `{"data":{"organization":{"id":"org-id","slug":"team","rawSlug":"team"}}}`,
		},
		{
			name: "server token fallback", slug: "team", canonical: "team", serverTokens: true,
			response: `{"data":{"organization":{"id":"org-id","slug":"team","rawSlug":"team"}}}`,
		},
		{
			name: "personal alias", slug: "personal", canonical: "personal",
			response: `{"data":{"organization":{"id":"org-id","slug":"personal","rawSlug":"my-personal-org"}}}`,
		},
		{
			name: "personal raw slug", slug: "my-personal-org", canonical: "personal",
			response: `{"data":{"organization":{"id":"org-id","slug":"personal","rawSlug":"my-personal-org"}}}`,
		},
		{
			name: "missing organization", slug: "missing",
			response: `{"data":{"organization":null}}`, wantError: errNoSuchOrg.Error(),
		},
		{
			name: "unauthorized organization", slug: "other",
			response:  `{"errors":[{"message":"Not authorized to access this organization"}]}`,
			wantError: "Not authorized to access this organization",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			wantToken := "fo1_session"
			if tc.serverTokens {
				wantToken = "fo1_server"
			}
			api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "Bearer "+wantToken, r.Header.Get("Authorization"))
				var request struct {
					Query     string
					Variables struct{ Slug string }
				}
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Error(err)
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				// Model a token that can read its org but cannot list all orgs.
				if strings.Contains(request.Query, "organizations(") {
					io.WriteString(w, `{"errors":[{"message":"Not authorized to access this organization"}]}`)
					return
				}
				assert.Contains(t, request.Query, "organization(slug: $slug)")
				assert.Equal(t, tc.slug, request.Variables.Slug)
				io.WriteString(w, tc.response)
			}))
			defer api.Close()
			fly.SetBaseURL(api.URL)
			defer fly.SetBaseURL("")

			srv := &server{tokens: tokens.Parse("fo1_server"), slugAliases: map[string]string{}}
			sess := &session{srv: srv}
			if !tc.serverTokens {
				sess.tokens = tokens.Parse("fo1_session")
			}
			org, err := sess.fetchOrg(context.Background(), tc.slug)
			if tc.wantError != "" {
				require.ErrorContains(t, err, tc.wantError)
				require.Nil(t, org)
				require.Empty(t, srv.slugAliases)
				return
			}
			require.NoError(t, err)
			require.Equal(t, "org-id", org.ID)
			require.Equal(t, tc.canonical, org.Slug)
			require.Equal(t, tc.canonical, srv.slugAliases[tc.slug])
			if tc.canonical == "personal" {
				require.Equal(t, "my-personal-org", org.RawSlug, "token gateway needs the real slug")
			}
		})
	}
}
