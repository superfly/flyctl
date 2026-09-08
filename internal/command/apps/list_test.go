package apps

import (
	"context"
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
				GetAllAppsCurrentReleaseTimestampsFunc: func(ctx context.Context) (*map[string]time.Time, error) {
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
