package postgres

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
