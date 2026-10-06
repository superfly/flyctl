package postgres

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/agent"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/mock"
	"github.com/superfly/flyctl/internal/prompt"
	"github.com/superfly/flyctl/iostreams"
	"github.com/superfly/flyctl/wg"
)

// testDialer sends every connection to addr, standing in for the WireGuard
// tunnel to the postgres leader.
type testDialer struct{ addr string }

func (testDialer) State() *wg.WireGuardState { return nil }
func (testDialer) Config() *wg.Config        { return nil }

func (d testDialer) DialContext(ctx context.Context, network, _ string) (net.Conn, error) {
	var dialer net.Dialer
	return dialer.DialContext(ctx, network, d.addr)
}

func TestRunAttachClusterExistingDatabaseWhenNotInteractive(t *testing.T) {
	// The leader reports that the consuming app's database already exists.
	leader := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/commands/databases/my_app" {
			http.NotFound(w, r)
			return
		}
		fmt.Fprint(w, `{"result":{"name":"my_app"}}`)
	}))
	defer leader.Close()

	ios, _, _, _ := iostreams.Test()
	ctx := iostreams.NewContext(context.Background(), ios)
	ctx = agent.DialerWithContext(ctx, testDialer{addr: leader.Listener.Addr().String()})
	ctx = flapsutil.NewContextWithClient(ctx, &mock.FlapsClient{
		ListAppSecretsFunc: func(context.Context, string, *uint64, bool) ([]fly.AppSecret, error) {
			return nil, nil
		},
	})

	err := runAttachCluster(ctx, "fdaa::2", AttachParams{AppName: "my-app", PgAppName: "my-app-db"}, nil)

	require.ErrorIs(t, err, prompt.ErrNonInteractive)
	assert.Contains(t, err.Error(), "--yes")
}
