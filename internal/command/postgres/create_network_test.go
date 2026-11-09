package postgres

import (
	"context"
	"testing"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/uiex"
)

func TestCreateNetworkFlag(t *testing.T) {
	cmd := newCreate()
	require.NoError(t, cmd.Flags().Parse([]string{"--network", "pgnet"}))

	got, err := cmd.Flags().GetString("network")
	require.NoError(t, err)
	assert.Equal(t, "pgnet", got)
}

func TestCreateNetworkFlagDefaultsEmpty(t *testing.T) {
	cmd := newCreate()
	require.NoError(t, cmd.Flags().Parse(nil))

	// An empty network keeps the cluster on the org's default network.
	got, err := cmd.Flags().GetString("network")
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestCreateNetworkFlagHelpSaysName(t *testing.T) {
	// The API takes a network name, and an unknown name creates a new network,
	// so the help must not suggest passing an ID.
	usage := newCreate().Flags().Lookup("network").Usage
	assert.Contains(t, usage, "Name of the custom private network")
	assert.NotContains(t, usage, " id")
}

func TestNewClusterInputNetworkFromParams(t *testing.T) {
	org := &uiex.Organization{Slug: "my-org"}
	region := &fly.Region{Code: "iad"}

	// The calling command defines its own --network (as fly launch might); the
	// cluster must still use only what the caller put in ClusterParams.
	fs := pflag.NewFlagSet("caller", pflag.ContinueOnError)
	fs.String("network", "", "")
	require.NoError(t, fs.Parse([]string{"--network", "from-flag"}))
	ctx := flag.NewContext(context.Background(), fs)

	for name, tc := range map[string]struct {
		network string
	}{
		"custom":  {network: "pgnet"},
		"default": {network: ""},
	} {
		t.Run(name, func(t *testing.T) {
			params := &ClusterParams{Network: tc.network}
			params.Name = "my-pg"

			input := newClusterInput(ctx, org, region, params)
			assert.Equal(t, tc.network, input.Network)
			assert.Equal(t, "my-pg", input.AppName)
			assert.Equal(t, "iad", input.Region)
		})
	}
}
