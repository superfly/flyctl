package apps

import (
	"context"
	"errors"
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
