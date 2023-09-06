package buildinfo

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestReleaseMeta(t *testing.T) {
	environment = "production"
	buildVersion = "1.2.3"
	buildDate = "2020-06-05T13:32:23Z"

	loadMeta()

	assert.Equal(t, "1.2.3", Version().String())
	assert.Equal(t, "2020-06-05T13:32:23Z", BuildTime().Format(time.RFC3339))
}

func TestBuildDate(t *testing.T) {
	loadMeta()
}

func TestDevMeta(t *testing.T) {
	environment = "development"
	buildVersion = "<version>"
	buildDate = "<date>"

	loadMeta()

	// check that date was set to time.Now() with a little leeway
	assert.WithinDuration(t, time.Now(), cachedBuildTime, 1*time.Millisecond)
	assert.Equal(t, fmt.Sprintf("0.0.0-dev.%d", cachedBuildTime.Unix()), Version().String())
}
