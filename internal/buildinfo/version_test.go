package buildinfo

import (
	"testing"
	"time"

	"github.com/loadsmart/calver-go/calver"
	"github.com/stretchr/testify/assert"
)

func TestProdMeta(t *testing.T) {
	environment = "production"
	version = "1.2.3"
	buildDate = "2020-06-05T13:32:23Z"

	loadMeta()

	assert.Equal(t, "1.2.3", ParsedVersion().String())
	assert.Equal(t, "2020-06-05T13:32:23Z", BuildDate().Format(time.RFC3339))
}

func TestDevMeta(t *testing.T) {
	environment = "development"
	version = "<version>"
	buildDate = "2020-06-05T13:32:23Z"

	loadMeta()

	current, _ := calver.NewVersion(calverFormat, int(BuildDate().Unix()))
	assert.Equal(t, current.String(), ParsedVersion().String())
}

func TestNewerSemver(t *testing.T) {
	environment = "production"
	version = "1.2.3"

	loadMeta()

	assert.Equal(t, "1.2.3", ParsedVersion().String())
	v, _ := ParseVersion("1.2.4")
	assert.True(t, v.Newer())
}
