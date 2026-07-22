package dockerfileurl

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsURL(t *testing.T) {
	assert.True(t, IsURL("https://example.com/Dockerfile"))
	assert.True(t, IsURL("HTTPS://example.com/Dockerfile"))
	assert.False(t, IsURL("Dockerfile.custom"))
	assert.False(t, IsURL("ftp://example.com/Dockerfile"))
}

func TestForDisplay(t *testing.T) {
	dockerfileURL := "https://" + "user:password@" + "example.com/path/Dockerfile?token=secret#fragment"

	assert.Equal(t, "Dockerfile.custom", ForDisplay("Dockerfile.custom"))
	assert.Equal(t, "https://example.com/path/Dockerfile", ForDisplay(dockerfileURL))
}
