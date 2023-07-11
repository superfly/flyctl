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

	v, _ := calver.NewVersion(calverFormat, int(BuildDate().Unix()))
	current := &CalverVersion{Version: *v}
	assert.Equal(t, current.String(), ParsedVersion().String())
}

func TestNewerSemver(t *testing.T) {
	environment = "production"
	version = "1.2.3"

	loadMeta()

	assert.Equal(t, "1.2.3", ParsedVersion().String())
	v, _ := ParseVersion("1.2.4")
	assert.True(t, v.Newer())
	v, _ = ParseVersion("1.2.2")
	assert.False(t, v.Newer())
}

func TestNewerCalver(t *testing.T) {
	environment = "production"
	version = "2023.06.30.1"

	loadMeta()

	assert.Equal(t, "2023.06.30-1", ParsedVersion().String())
	v, _ := ParseVersion("2023.07.01.1")
	assert.True(t, v.Newer())
	v, _ = ParseVersion("2023.06.29.1")
	assert.False(t, v.Newer())
}

func TestCalverAlwaysNewer(t *testing.T) {
	environment = "production"
	version = "1.2.3"

	loadMeta()

	assert.Equal(t, "1.2.3", ParsedVersion().String())
	v, _ := ParseVersion("2023.07.01.1")
	assert.True(t, v.Newer())
}

func TestDashedCalver(t *testing.T) {
	version = "2023.06.30-1"
	buildDate = "2020-06-05T13:32:23Z"

	loadMeta()

	assert.Equal(t, "2023.06.30-1", ParsedVersion().String())
}

func TestDashedCalverRejectsSemver(t *testing.T) {
	version = "1.2.3-1"
	buildDate = "2020-06-05T13:32:23Z"

	loadMeta()

	assert.Equal(t, "1.2.3-1", ParsedVersion().String())
	_, ok := ParsedVersion().(*CalverVersion)
	assert.False(t, ok)
}
