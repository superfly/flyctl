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

func TestLooksLikeURL(t *testing.T) {
	assert.True(t, LooksLikeURL("https://example.com/%zz"))
	assert.True(t, LooksLikeURL("HTTPS://example.com/%zz"))
	assert.True(t, LooksLikeURL("https:/example.com/%zz"))
	assert.True(t, LooksLikeURL(`https:\example.com\Dockerfile`))
	assert.False(t, LooksLikeURL("Dockerfile.custom"))
}

func TestForDisplay(t *testing.T) {
	dockerfileURL := "https://" + "user:password@" + "example.com/path/Dockerfile?token=secret#fragment"

	assert.Equal(t, "Dockerfile.custom", ForDisplay("Dockerfile.custom"))
	assert.Equal(t, "https://example.com/path/Dockerfile", ForDisplay(dockerfileURL))
	assert.Equal(t, "invalid URL", ForDisplay("https://example.com/%zz?token=secret"))
	assert.Equal(t, "invalid URL", ForDisplay("https:/example.com/%zz?token=secret"))
	assert.Equal(t, "invalid URL", ForDisplay(`https:\example.com\Dockerfile?token=secret`))
}

func TestForRequestError(t *testing.T) {
	assert.Equal(t, "/Dockerfile", ForRequestError("/Dockerfile?token=secret#fragment"))
	assert.Equal(t, "invalid URL", ForRequestError("https://example.com/%zz?token=secret"))
}
