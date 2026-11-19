package flapsutil

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNormalizeNetworkName(t *testing.T) {
	for in, want := range map[string]string{
		"":          "",
		"   ":       "",
		"default":   "",
		" default ": "",
		"pgnet":     "pgnet",
		" pgnet\n":  "pgnet",
	} {
		assert.Equal(t, want, NormalizeNetworkName(in), "NormalizeNetworkName(%q)", in)
	}
}
