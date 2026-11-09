package apps

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCreateNetworkFlagHelpSaysName(t *testing.T) {
	// The API takes a network name, and an unknown name creates a new network.
	usage := newCreate().Flags().Lookup("network").Usage
	assert.Contains(t, usage, "Name of the custom private network")
	assert.Contains(t, usage, "doesn't exist yet is created")
}
