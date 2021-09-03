package buildinfo

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestProdMeta(t *testing.T) {
	environment = "production"
	version = "1.2.3"
	buildDate = "2020-06-05T13:32:23Z"
	commit = "c8f7b8f"

	loadMeta()

	assert.Equal(t, "1.2.3", Version().String())
	assert.Equal(t, "2020-06-05T13:32:23Z", BuildDate().Format(time.RFC3339))
	assert.Equal(t, "c8f7b8f", Commit())
}

func TestDevMeta(t *testing.T) {
	environment = "development"
	version = "<version>"
	commit = "<commit>"

	loadMeta()

	assert.Equal(t, fmt.Sprintf("0.0.0-%d+dev", BuildDate().Unix()), Version().String())
	assert.Equal(t, "<commit>", Commit())
}
