package apps

import (
	"context"
	"errors"
	"fmt"
	"github.com/superfly/fly-go/tokens"
	"github.com/superfly/macaroon"
	"github.com/superfly/macaroon/flyio"
	"github.com/superfly/macaroon/resset"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/require"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/mock"
	"github.com/superfly/flyctl/internal/uiex"
	"github.com/superfly/flyctl/internal/uiexutil"
	"github.com/superfly/flyctl/iostreams"
)

func TestRunListEmptyApps(t *testing.T) {
	tests := []struct {
		name       string
		jsonOutput bool
		quiet      bool
		want       string
	}{
		{
			name: "default",
			want: "No apps found\n",
		},
		{
			name:  "quiet",
			quiet: true,
			want:  "",
		},
		{
			name:       "JSON",
			jsonOutput: true,
			want:       "[]\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			io, _, out, _ := iostreams.Test()
			ctx := iostreams.NewContext(context.Background(), io)
			ctx = config.NewContext(ctx, &config.Config{JSONOutput: tt.jsonOutput})
			flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
			flags.Bool("quiet", tt.quiet, "")
			flags.String("org", "", "")
			ctx = flag.NewContext(ctx, flags)
			ctx = uiexutil.NewContextWithClient(ctx, &mock.UiexClient{
				ListOrganizationsFunc: func(ctx context.Context, admin bool) ([]uiex.Organization, error) {
					return []uiex.Organization{{Slug: "personal", RawSlug: "raw-personal", Personal: true}}, nil
				},
				GetAllAppsCurrentReleaseTimestampsFunc: func(ctx context.Context, orgSlug string) (*map[string]time.Time, error) {
					return &map[string]time.Time{}, nil
				},
			})
			ctx = flapsutil.NewContextWithClient(ctx, &mock.FlapsClient{
				ListAppsFunc: func(ctx context.Context, req flaps.ListAppsRequest) ([]flaps.App, error) {
					require.Equal(t, "personal", req.OrgSlug)

					return nil, nil
				},
			})

			require.NoError(t, runList(ctx))
			require.Equal(t, tt.want, out.String())
		})
	}
}

// The current-release timestamps only fill the "Latest Deploy" column. When
// --org is set the request is narrowed to that org's raw slug, and if it
// fails the apps are still listed, without deploy times.
func TestRunListReleaseTimestamps(t *testing.T) {
	tests := []struct {
		name         string
		org          string
		wantOrgSlug  string
		timestampErr error
		want         string
	}{
		{
			name:        "org flag forwards the raw slug",
			org:         "personal",
			wantOrgSlug: "raw-personal",
			want: " NAME │ OWNER    │ STATUS   │ LATEST DEPLOY     \n" +
				" one  │ personal │ deployed │ Nov 14 2025 17:09 \n\n",
		},
		{
			name:        "no org flag asks for every app",
			wantOrgSlug: "",
			want: " NAME │ OWNER    │ STATUS   │ LATEST DEPLOY     \n" +
				" one  │ personal │ deployed │ Nov 14 2025 17:09 \n\n",
		},
		{
			name:         "timestamp failure still lists apps",
			org:          "personal",
			wantOrgSlug:  "raw-personal",
			timestampErr: errors.New("boom"),
			want: " NAME │ OWNER    │ STATUS   │ LATEST DEPLOY \n" +
				" one  │ personal │ deployed │               \n\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			io, _, out, _ := iostreams.Test()
			ctx := iostreams.NewContext(context.Background(), io)
			ctx = config.NewContext(ctx, &config.Config{})
			flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
			flags.Bool("quiet", false, "")
			flags.String("org", tt.org, "")
			ctx = flag.NewContext(ctx, flags)

			org := uiex.Organization{Slug: "personal", RawSlug: "raw-personal", Personal: true}
			var gotOrgSlug string
			ctx = uiexutil.NewContextWithClient(ctx, &mock.UiexClient{
				ListOrganizationsFunc: func(ctx context.Context, admin bool) ([]uiex.Organization, error) {
					return []uiex.Organization{org}, nil
				},
				GetOrganizationFunc: func(ctx context.Context, orgSlug string) (*uiex.Organization, error) {
					require.Equal(t, "personal", orgSlug)
					return &org, nil
				},
				GetAllAppsCurrentReleaseTimestampsFunc: func(ctx context.Context, orgSlug string) (*map[string]time.Time, error) {
					gotOrgSlug = orgSlug
					if tt.timestampErr != nil {
						return nil, tt.timestampErr
					}
					return &map[string]time.Time{"one": time.Date(2025, 11, 14, 17, 9, 53, 0, time.UTC)}, nil
				},
			})
			ctx = flapsutil.NewContextWithClient(ctx, &mock.FlapsClient{
				ListAppsFunc: func(ctx context.Context, req flaps.ListAppsRequest) ([]flaps.App, error) {
					require.Equal(t, "personal", req.OrgSlug)
					return []flaps.App{{
						Name:         "one",
						Status:       "deployed",
						Organization: flaps.AppOrganizationInfo{Slug: "raw-personal", Name: "personal"},
					}}, nil
				},
			})

			require.NoError(t, runList(ctx))
			require.Equal(t, tt.wantOrgSlug, gotOrgSlug)
			require.Equal(t, tt.want, out.String())
		})
	}
}

// Scope each org's request without narrowing discovery or the global timestamp
// listing. Exercise real client headers, not only the context passed to a mock.
func TestListAppsRequestCredentials(t *testing.T) {
	var roots []string
	for _, id := range []uint64{1, 2} {
		m, err := macaroon.New([]byte("local"), flyio.LocationPermission, macaroon.NewSigningKey())
		require.NoError(t, err)
		require.NoError(t, m.Add(&flyio.Organization{ID: id, Mask: resset.ActionAll}))
		root, err := m.String()
		require.NoError(t, err)
		roots = append(roots, root)
	}
	for _, only := range []string{"", "personal"} {
		t.Run("org="+only, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				header := r.Header.Get("Authorization")
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case "/api/v1/organizations", "/api/v1/organizations/personal":
					for _, root := range roots {
						require.Contains(t, header, root)
					}
					if only == "" {
						fmt.Fprint(w, `{"organizations":[{"id":"one","internal_numeric_id":1,"slug":"personal","raw_slug":"one","personal":true},{"id":"two","internal_numeric_id":2,"slug":"two","raw_slug":"two"}]}`)
					} else {
						fmt.Fprint(w, `{"organization":{"id":"one","internal_numeric_id":1,"slug":"personal","raw_slug":"one","personal":true}}`)
					}
				case "/v1/apps":
					index := 0
					if r.URL.Query().Get("org_slug") == "two" {
						index = 1
					}
					require.Contains(t, header, roots[index])
					require.NotContains(t, header, roots[1-index])
					fmt.Fprintf(w, `{"apps":[{"id":"%d","name":"app%d","organization":{"slug":"%s"}}]}`, index, index, []string{"one", "two"}[index])
				default:
					require.Contains(t, header, roots[0])
					if only == "" {
						require.Contains(t, header, roots[1])
					} else {
						require.NotContains(t, header, roots[1])
					}
					fmt.Fprint(w, `{}`)
				}
			}))
			defer server.Close()
			t.Setenv("FLY_FLAPS_BASE_URL", server.URL)
			endpoint, err := url.Parse(server.URL)
			require.NoError(t, err)
			tok := tokens.Parse(strings.Join(roots, ","))
			ctx := config.NewContext(context.Background(), &config.Config{Tokens: tok})
			f, err := flapsutil.NewClientWithOptions(ctx, flaps.NewClientOpts{Tokens: tok})
			require.NoError(t, err)
			u, err := uiex.NewWithOptions(ctx, uiex.NewClientOpts{BaseURL: endpoint, Tokens: tok})
			require.NoError(t, err)
			ctx = flapsutil.NewContextWithClient(ctx, f)
			ctx = uiexutil.NewContextWithClient(ctx, u)
			apps, err := getApps(ctx, only)
			require.NoError(t, err)
			want := 2
			if only != "" {
				want = 1
			}
			require.Len(t, apps, want)
			require.Equal(t, want+2, calls)
		})
	}
}
