package deploy

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestContainsCommonSecretSubstring(t *testing.T) {
	assert.True(t, containsCommonSecretSubstring("THIRDPARTY_SERVICE_SECRET"))
}
