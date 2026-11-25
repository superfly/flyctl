package postgres

import (
	"testing"

	"github.com/stretchr/testify/require"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/uiex"
)

func TestResolveRestoreTargetInclusive(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{"no target", []string{"--restore-target-inclusive=false"}, ""},
		{"default", []string{"--restore-target-name", "base"}, "?targetName=base&targetInclusive=true"},
		{"exclusive", []string{"--restore-target-time", "2021-07-16T12:34:56Z", "--restore-target-inclusive=false"}, "?targetTime=2021-07-16T12:34:56Z&targetInclusive=false"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmd := newBackupRestore()
			require.NoError(t, cmd.ParseFlags(tc.args))
			require.Equal(t, tc.want, resolveRestoreTarget(flag.NewContext(t.Context(), cmd.Flags())))
		})
	}
}

func TestNewClusterInputConsulURL(t *testing.T) {
	cmd := newCreate()
	require.NoError(t, cmd.ParseFlags([]string{"--consul-url", "https://consul.example"}))
	ctx := flag.NewContext(t.Context(), cmd.Flags())

	input := newClusterInput(ctx, &uiex.Organization{Slug: "my-org"}, &fly.Region{Code: "iad"}, &ClusterParams{})
	require.Equal(t, "https://consul.example", input.ConsulURL)
}
