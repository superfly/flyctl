package cmdutil

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseKVStringsDoesNotEchoSensitiveInput(t *testing.T) {
	_, err := ParseKVStringsToMap([]string{"VALID=value", "sensitive-value-without-a-name"})
	require.EqualError(t, err, "argument 2 must be in the format NAME=VALUE")
	require.NotContains(t, err.Error(), "sensitive-value")
}
