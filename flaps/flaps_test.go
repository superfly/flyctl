package flaps

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSnakeCase(t *testing.T) {
	assert.Equal(t, "foo_bar", snakeCase("fooBar"))
	assert.Equal(t, "app_create", snakeCase(appCreate.String()))
}
