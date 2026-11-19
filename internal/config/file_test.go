package config

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSetAccessTokenRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yml")

	require.NoError(t, SetAccessToken(path, "fo1_token"))

	token, err := ReadAccessToken(path)
	require.NoError(t, err)
	require.Equal(t, "fo1_token", token)
}
