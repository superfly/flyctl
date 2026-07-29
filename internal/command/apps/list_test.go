package apps

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/require"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flyutil"
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
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, http.MethodPost, r.Method)
				require.Equal(t, "/graphql", r.URL.Path)
				fmt.Fprint(w, `{"data":{"apps":{"pageInfo":{"hasNextPage":false,"endCursor":null},"nodes":[]}}}`)
			}))
			t.Cleanup(server.Close)

			io, _, out, _ := iostreams.Test()
			ctx := iostreams.NewContext(context.Background(), io)
			ctx = config.NewContext(ctx, &config.Config{JSONOutput: tt.jsonOutput})
			flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
			flags.Bool("quiet", tt.quiet, "")
			ctx = flag.NewContext(ctx, flags)
			ctx = flyutil.NewContextWithClient(ctx, flyutil.NewClientFromOptions(ctx, fly.ClientOptions{BaseURL: server.URL}))

			require.NoError(t, runList(ctx))
			require.Equal(t, tt.want, out.String())
		})
	}
}
