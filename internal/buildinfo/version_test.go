package buildinfo

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestProdMeta(t *testing.T) {
	version = "1.2.3"
	buildDate = "2020-06-05T13:32:23Z"
	commit = "c8f7b8f"

	loadMeta(time.Now())

	assert.Equal(t, "1.2.3", Version().String())
	assert.Equal(t, "2020-06-05T13:32:23Z", BuildDate().Format(time.RFC3339))
	assert.Equal(t, "c8f7b8f", Commit())
}

func TestDevMeta(t *testing.T) {
	version = "<version>"
	buildDate = "<date>"
	commit = "<commit>"

	now := time.Now()
	loadMeta(now)

	assert.Equal(t, fmt.Sprintf("0.0.0-%d+dev", now.Unix()), Version().String())
	assert.Equal(t, now, BuildDate())
	assert.Equal(t, "<commit>", Commit())
}
