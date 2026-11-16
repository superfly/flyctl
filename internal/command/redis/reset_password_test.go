package redis

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Without the provider the API finds the add-on by name alone, which can be
// another provider's add-on with the same name, such as a Tigris bucket.
func TestResetPasswordSendsProvider(t *testing.T) {
	client := &captureGenqClient{}

	_, err := resetPassword(context.Background(), client.client(), "cache")
	require.NoError(t, err)

	assert.Equal(t, []string{"ResetAddOnPassword"}, client.opNames)
	assert.Equal(t, "cache", client.variables["name"])
	assert.Equal(t, "upstash_redis", client.variables["provider"])
}
