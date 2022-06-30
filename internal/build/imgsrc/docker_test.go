package imgsrc

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAllowedDockerDaemonMode(t *testing.T) {
	tests := []struct {
		allowLocal  bool
		allowRemote bool
		expected    BuilderType
	}{
		{true, true, BuilderTypeNone | BuilderTypeLocal | BuilderTypeRemote},
		{false, true, BuilderTypeNone | BuilderTypeRemote},
		{true, false, BuilderTypeNone | BuilderTypeLocal},
		{false, false, BuilderTypeNone},
	}

	for _, test := range tests {
		m := NewDockerDaemonType(test.allowLocal, test.allowRemote, false)
		assert.Equal(t, test.expected, m)
	}
}
