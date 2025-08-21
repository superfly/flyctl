package imgsrc

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/superfly/fly-go"
	"github.com/superfly/flyctl/internal/cache"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/iostreams"
)

func TestInitBuilder(t *testing.T) {
	ctx := context.Background()
	ctx = cache.NewContext(ctx, cache.New())
	ctx = flyutil.NewContextWithClient(ctx, flyutil.NewClientFromOptions(ctx, fly.ClientOptions{BaseURL: "invalid://localhost"}))
	ios, _, _, _ := iostreams.Test()
	build := newBuild("build1", false)

	// The invocation below doesn't test things much, but it may be better than nothing.
	_, _, err := initBuilder(ctx, build, "app1", ios, DepotBuilderScopeOrganization)
	require.ErrorContains(t, err, `unsupported protocol scheme "invalid"`)
}
