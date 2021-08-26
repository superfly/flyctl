package imgsrc

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAllowedDockerDaemonMode(t *testing.T) {
	tests := []struct {
		allowLocal  bool
		allowRemote bool
		expected    DockerDaemonType
	}{
		{true, true, DockerDaemonTypeNone | DockerDaemonTypeLocal | DockerDaemonTypeRemote},
		{false, true, DockerDaemonTypeNone | DockerDaemonTypeRemote},
		{true, false, DockerDaemonTypeNone | DockerDaemonTypeLocal},
		{false, false, DockerDaemonTypeNone},
	}

	for _, test := range tests {
		m := NewDockerDaemonType(test.allowLocal, test.allowRemote, false)
		assert.Equal(t, test.expected, m)
	}
}
