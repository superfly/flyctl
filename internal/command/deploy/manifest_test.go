package deploy

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestManifest(t *testing.T) {
	m := NewManifest("app1", nil, MachineDeploymentArgs{})
	var buf bytes.Buffer
	m.Encode(&buf)
	assert.Contains(t, buf.String(), "{")
}
