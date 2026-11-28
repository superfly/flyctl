package mcp

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestVolumeConfigFlag(t *testing.T) {
	cmd := newVolume()
	err := cmd.Flags().Parse([]string{"--config", "/path/to/claude.json", "--config", "/path/to/cursor.json"})
	require.NoError(t, err)

	configs, err := cmd.Flags().GetStringArray("config")
	require.NoError(t, err)
	require.Equal(t, []string{"/path/to/claude.json", "/path/to/cursor.json"}, configs)
}
