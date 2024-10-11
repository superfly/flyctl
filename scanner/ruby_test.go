package scanner

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRubyVersionParsing(t *testing.T) {
	v := extractGemfileRuby([]byte(`
		source "https://rubygems.org"

		ruby '3.1.0'
	`))

	require.Equal(t, v, "3.1.0")

	v = extractGemfileRuby([]byte(`ruby "3.1.0"`))

	require.Equal(t, v, "3.1.0")
}
