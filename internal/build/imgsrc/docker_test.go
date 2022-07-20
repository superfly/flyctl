package imgsrc

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAllowedDockerDaemonMode(t *testing.T) {
	tests := []struct {
		allowLocal   bool
		allowRemote  bool
		preferslocal bool
		expected     DockerDaemonType
	}{
		{false, false, false, DockerDaemonTypeNone},
		{false, false, true, DockerDaemonTypeNone | DockerDaemonTypePrefersLocal},
		{false, true, false, DockerDaemonTypeNone | DockerDaemonTypeRemote},
		{false, true, true, DockerDaemonTypeNone | DockerDaemonTypeRemote | DockerDaemonTypePrefersLocal},
		{true, false, false, DockerDaemonTypeNone | DockerDaemonTypeLocal},
		{true, false, true, DockerDaemonTypeNone | DockerDaemonTypeLocal | DockerDaemonTypePrefersLocal},
		{true, true, false, DockerDaemonTypeNone | DockerDaemonTypeLocal | DockerDaemonTypeRemote},
		{true, true, true, DockerDaemonTypeNone | DockerDaemonTypeLocal | DockerDaemonTypeRemote | DockerDaemonTypePrefersLocal},
	}

	for _, test := range tests {
		m := NewDockerDaemonType(test.allowLocal, test.allowRemote, test.preferslocal)
		assert.Equal(t, test.expected, m)
	}
}
