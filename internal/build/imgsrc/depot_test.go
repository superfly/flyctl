package imgsrc

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/superfly/fly-go"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/mock"
	"github.com/superfly/flyctl/internal/uiex"
	"github.com/superfly/flyctl/internal/uiexutil"
	"github.com/superfly/flyctl/iostreams"
)

func TestInitBuilder(t *testing.T) {
	ctx := context.Background()
	ctx = flyutil.NewContextWithClient(ctx, flyutil.NewClientFromOptions(ctx, fly.ClientOptions{BaseURL: "invalid://localhost"}))

	mockUiex := &mock.UiexClient{
		EnsureDepotBuilderFunc: func(ctx context.Context, in uiex.EnsureDepotBuilderRequest) (*uiex.EnsureDepotBuilderResponse, error) {
			return nil, errors.New(`unsupported protocol scheme "invalid"`)
		},
	}
	ctx = uiexutil.NewContextWithClient(ctx, mockUiex)

	ios, _, _, _ := iostreams.Test()
	build := newBuild(1, false)

	// The invocation below doesn't test things much, but it may be better than nothing.
	_, _, err := initBuilder(ctx, build, "app1", ios, DepotBuilderScopeOrganization)
	require.ErrorContains(t, err, `unsupported protocol scheme "invalid"`)
}

func TestDepotBuilderRegion(t *testing.T) {
	cases := []struct {
		name      string
		envRegion string
		altRegion string
		want      string
	}{
		{name: "no region", envRegion: "", altRegion: "", want: ""},
		{name: "env only", envRegion: "iad", altRegion: "", want: "fly-iad"},
		{name: "alt only", envRegion: "", altRegion: "ord", want: "fly-ord"},
		{name: "env wins over alt", envRegion: "iad", altRegion: "ord", want: "fly-iad"},
		{name: "alias alt region", envRegion: "", altRegion: "eu", want: "fly-eu"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, depotBuilderRegion(tc.envRegion, tc.altRegion))
		})
	}
}
