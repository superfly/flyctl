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
		useDepot     bool
		useNixpacks  bool
		expected     DockerDaemonType
	}{
		{false, false, false, false, false, DockerDaemonTypeNone},
		{false, false, true, false, false, DockerDaemonTypeNone | DockerDaemonTypePrefersLocal},
		{false, true, false, false, false, DockerDaemonTypeNone | DockerDaemonTypeRemote},
		{false, true, true, false, false, DockerDaemonTypeNone | DockerDaemonTypeRemote | DockerDaemonTypePrefersLocal},
		{true, false, false, false, false, DockerDaemonTypeNone | DockerDaemonTypeLocal},
		{true, false, true, false, false, DockerDaemonTypeNone | DockerDaemonTypeLocal | DockerDaemonTypePrefersLocal},
		{true, true, false, false, false, DockerDaemonTypeNone | DockerDaemonTypeLocal | DockerDaemonTypeRemote},
		{true, true, true, false, false, DockerDaemonTypeNone | DockerDaemonTypeLocal | DockerDaemonTypeRemote | DockerDaemonTypePrefersLocal},
		{true, true, false, true, false, DockerDaemonTypeNone | DockerDaemonTypeDepot | DockerDaemonTypeRemote | DockerDaemonTypeLocal},
	}

	for _, test := range tests {
		m := NewDockerDaemonType(test.allowLocal, test.allowRemote, test.preferslocal, test.useDepot, test.useNixpacks)
		assert.Equal(t, test.expected, m)
	}
}
