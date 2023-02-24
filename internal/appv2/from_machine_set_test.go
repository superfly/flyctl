package appv2

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestQuotePosixWords(t *testing.T) {
	type test struct {
		input    []string
		expected []string
	}
	tests := []test{
		{input: []string{
			"nginx", "-g", "daemon off;",
		}, expected: []string{
			"nginx", "-g", "'daemon off;'",
		}},
		{input: []string{
			"",
		}, expected: []string{
			"",
		}},
		{input: []string{
			"/app",
		}, expected: []string{
			"/app",
		}},
		{input: []string{
			"bundle", "exec", "rake", "db:migrate",
		}, expected: []string{
			"bundle", "exec", "rake", "db:migrate",
		}},
		{input: []string{
			"/release_cmd.sh", "foo", "bar", "baz", "123", "--six=seven",
		}, expected: []string{
			"/release_cmd.sh", "foo", "bar", "baz", "123", "'--six=seven'",
		}},
		{input: []string{
			"echo", "hi there",
		}, expected: []string{
			"echo", `"hi there"`,
		}},
	}
	for _, tc := range tests {
		result := quotePosixWords(tc.input)
		require.EqualValues(t, tc.expected, result)
	}
}
