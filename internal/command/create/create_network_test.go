package create

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCreateNetworkFlagHelpSaysName(t *testing.T) {
	// fly create is an alias of fly apps create and must describe --network the same way.
	usage := New().Flags().Lookup("network").Usage
	assert.Contains(t, usage, "Name of the custom private network")
}
