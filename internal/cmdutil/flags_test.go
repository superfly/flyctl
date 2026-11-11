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

func TestParseKVFlagNamesFlag(t *testing.T) {
	_, err := ParseKVFlagToMap("label", []string{"A=1", "sensitive-value"})
	require.EqualError(t, err, "--label value 2 must be in the format NAME=VALUE")
	require.NotContains(t, err.Error(), "sensitive-value")

	got, err := ParseKVFlagToMap("env", []string{"A=1", "B=x=y", "C="})
	require.NoError(t, err)
	require.Equal(t, map[string]string{"A": "1", "B": "x=y", "C": ""}, got)
}
