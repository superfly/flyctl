package secrets

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_parse_basic(t *testing.T) {
	reader := strings.NewReader(`
# A comment plus a new line with spaces

FOO=BAR
# Another comment
QUX=NAH
`)
	secrets, err := parseSecrets(reader)
	assert.NoError(t, err)
	assert.Equal(t, map[string]string{
		"FOO": "BAR",
		"QUX": "NAH",
	}, secrets)
}

func Test_parse_unix(t *testing.T) {
	reader := strings.NewReader("FOO=BAR\nQUX=NAH\n")
	secrets, err := parseSecrets(reader)
	assert.NoError(t, err)
	assert.Equal(t, map[string]string{
		"FOO": "BAR",
		"QUX": "NAH",
	}, secrets)
}

func Test_parse_windows(t *testing.T) {
	reader := strings.NewReader("FOO=BAR\r\nQUX=NAH\r\n")
	secrets, err := parseSecrets(reader)
	assert.NoError(t, err)
	assert.Equal(t, map[string]string{
		"FOO": "BAR",
		"QUX": "NAH",
	}, secrets)
}

func Test_parse_mulltiline(t *testing.T) {
	reader := strings.NewReader(`
FOO=BAR
MULTILINE="""SOMETHING
ANOTHER LINE

ENDSHERE"""
TRAILERENV=what
FIN="""Here is the end,
my only friend"""
`)
	secrets, err := parseSecrets(reader)
	assert.NoError(t, err)
	assert.Equal(t, map[string]string{
		"FOO":        "BAR",
		"MULTILINE":  "SOMETHING\nANOTHER LINE\n\nENDSHERE",
		"TRAILERENV": "what",
		"FIN":        "Here is the end,\nmy only friend",
	}, secrets)
}

func Test_parse_with_comma(t *testing.T) {
	reader := strings.NewReader("FOO=BAR,BAZ")
	secrets, err := parseSecrets(reader)
	assert.NoError(t, err)
	assert.Equal(t, map[string]string{
		"FOO": "BAR,BAZ",
	}, secrets)
}

func Test_parse_with_equal(t *testing.T) {
	reader := strings.NewReader("FOO=BAR BAZ")
	secrets, err := parseSecrets(reader)
	assert.NoError(t, err)
	assert.Equal(t, map[string]string{
		"FOO": "BAR BAZ",
	}, secrets)
}

func Test_parse_with_double_quotes(t *testing.T) {
	reader := strings.NewReader(`FOO="BAR BAZ"`)
	secrets, err := parseSecrets(reader)
	assert.NoError(t, err)
	assert.Equal(t, map[string]string{
		"FOO": "BAR BAZ",
	}, secrets)
}

// https://github.com/superfly/flyctl/issues/3002
func Test_parse_with_spaces(t *testing.T) {
	reader := strings.NewReader(`FOO = BAR`)
	secrets, err := parseSecrets(reader)
	assert.NoError(t, err)
	assert.Equal(t, map[string]string{
		"FOO": "BAR",
	}, secrets)
}

// https://github.com/superfly/flyctl/issues/4291
func Test_parse_with_comment(t *testing.T) {
	reader := strings.NewReader(`FOO="BAR BAZ" # comment`)
	secrets, err := parseSecrets(reader)
	assert.NoError(t, err)
	assert.Equal(t, map[string]string{
		"FOO": "BAR BAZ",
	}, secrets)
}

func Test_parse_with_single_quotes(t *testing.T) {
	reader := strings.NewReader("FOO='BAR BAZ'\nKEY='value'")
	secrets, err := parseSecrets(reader)
	assert.NoError(t, err)
	assert.Equal(t, map[string]string{
		"FOO": "BAR BAZ",
		"KEY": "value",
	}, secrets)
}

// Test single-line triple-quoted strings
func Test_parse_singleline_triple_quotes(t *testing.T) {
	reader := strings.NewReader(`VARIABLE="""my-single-line-multiline-string"""
ANOTHER="""another"""`)
	secrets, err := parseSecrets(reader)
	assert.NoError(t, err)
	assert.Equal(t, map[string]string{
		"VARIABLE": "my-single-line-multiline-string",
		"ANOTHER":  "another",
	}, secrets)
}

// Test edge cases for triple quotes
func Test_parse_triple_quotes_edge_cases(t *testing.T) {
	reader := strings.NewReader(`EMPTY=""""""
SINGLE="""x"""
WITHSPACES="""  spaces  """
MIXED="""line1"""
NORMAL=regular`)
	secrets, err := parseSecrets(reader)
	assert.NoError(t, err)
	assert.Equal(t, map[string]string{
		"EMPTY":      "",
		"SINGLE":     "x",
		"WITHSPACES": "  spaces  ",
		"MIXED":      "line1",
		"NORMAL":     "regular",
	}, secrets)
}

// Test single-line triple quotes with spaces around equals
func Test_parse_singleline_triple_quotes_with_spaces(t *testing.T) {
	reader := strings.NewReader(`VARIABLE = """my-single-line-multiline-string"""
ANOTHER = """another"""`)
	secrets, err := parseSecrets(reader)
	assert.NoError(t, err)
	assert.Equal(t, map[string]string{
		"VARIABLE": "my-single-line-multiline-string",
		"ANOTHER":  "another",
	}, secrets)
}
