package imgsrc

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/superfly/fly-go"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/uiex"
	"github.com/superfly/flyctl/internal/uiexutil"
	"github.com/superfly/flyctl/iostreams"
)

func TestInitBuilder(t *testing.T) {
	ctx := context.Background()
	ctx = config.NewContext(ctx, &config.Config{
		Tokens: nil,
	})

	ctx = flyutil.NewContextWithClient(ctx, flyutil.NewClientFromOptions(ctx, fly.ClientOptions{BaseURL: "invalid://localhost"}))

	client, _ := uiex.NewWithOptions(ctx, uiex.NewClientOpts{BaseURL: "invalid://localhost"})
	ctx = uiexutil.NewContextWithClient(ctx, client)

	ios, _, _, _ := iostreams.Test()
	build := newBuild(1, false)

	// The invocation below doesn't test things much, but it may be better than nothing.
	_, _, err := initBuilder(ctx, build, "app1", ios, DepotBuilderScopeOrganization)
	require.ErrorContains(t, err, `unsupported protocol scheme "invalid"`)
}
