package secrets

import (
	"bufio"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func Test_parse_lone_quote(t *testing.T) {
	for _, quote := range []string{`"`, `'`} {
		for name, ending := range map[string]string{"EOF": "", "LF": "\n", "CRLF": "\r\n"} {
			t.Run(quote+"/"+name, func(t *testing.T) {
				secrets, err := parseSecrets(strings.NewReader("FOO=" + quote + ending))
				require.NoError(t, err)
				assert.Equal(t, map[string]string{"FOO": quote}, secrets)
			})
		}
	}
}

func Test_parse_unterminated_multiline(t *testing.T) {
	for _, tt := range []struct {
		name  string
		input string
	}{
		{name: "opening quotes at EOF", input: "KEY=\"\"\""},
		{name: "content at EOF", input: "KEY=\"\"\"line one"},
		{name: "multiple lines", input: "FOO=BAR\nKEY=\"\"\"line one\nline two\n"},
		{name: "CRLF", input: "FOO=BAR\r\nKEY=\"\"\"line one\r\nline two\r\n"},
		{name: "after closed multiline", input: "GOOD=\"\"\"one\ntwo\"\"\"\nKEY=\"\"\"unfinished"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			secrets, err := parseSecrets(strings.NewReader(tt.input))
			require.ErrorContains(t, err, "KEY")
			assert.Nil(t, secrets)
		})
	}
}

func Test_parse_line_too_long(t *testing.T) {
	for _, tt := range []struct {
		name  string
		input string
	}{
		{name: "first line", input: "FOO=" + strings.Repeat("a", 1<<20) + "\nBAR=baz\n"},
		{name: "after valid secret", input: "GOOD=value\nFOO=" + strings.Repeat("a", 1<<20)},
		{name: "multiline body", input: "GOOD=value\nFOO=\"\"\"\n" + strings.Repeat("a", 1<<20) + "\n\"\"\""},
		{name: "comment", input: "GOOD=value\n#" + strings.Repeat("a", 1<<20)},
	} {
		t.Run(tt.name, func(t *testing.T) {
			secrets, err := parseSecrets(strings.NewReader(tt.input))
			require.ErrorIs(t, err, bufio.ErrTooLong)
			assert.Nil(t, secrets)
		})
	}
}

func TestParseSecretsValidEdgeCases(t *testing.T) {
	for _, tt := range []struct {
		name  string
		input string
		want  map[string]string
	}{
		{name: "empty input", input: "", want: map[string]string{}},
		{name: "comments and whitespace", input: "# comment\r\n  \n", want: map[string]string{}},
		{name: "empty value", input: "FOO=", want: map[string]string{"FOO": ""}},
		{name: "empty double quotes", input: "FOO=\"\"", want: map[string]string{"FOO": ""}},
		{name: "empty single quotes", input: "FOO=''", want: map[string]string{"FOO": ""}},
		{name: "empty triple quotes", input: "FOO=\"\"\"\"\"\"", want: map[string]string{"FOO": ""}},
		{name: "one character double quoted", input: "FOO=\"x\"", want: map[string]string{"FOO": "x"}},
		{name: "one character single quoted", input: "FOO='x'", want: map[string]string{"FOO": "x"}},
		{name: "unmatched opening double quote", input: "FOO=\"x", want: map[string]string{"FOO": "\"x"}},
		{name: "unmatched closing double quote", input: "FOO=x\"", want: map[string]string{"FOO": "x\""}},
		{name: "unmatched opening single quote", input: "FOO='x", want: map[string]string{"FOO": "'x"}},
		{name: "unmatched closing single quote", input: "FOO=x'", want: map[string]string{"FOO": "x'"}},
		{name: "unicode", input: "FOO=\"héllo 世界\"", want: map[string]string{"FOO": "héllo 世界"}},
		{name: "equals in value", input: "FOO=a=b=c", want: map[string]string{"FOO": "a=b=c"}},
		{name: "quoted hash", input: "FOO=\"a#b\"", want: map[string]string{"FOO": "a#b"}},
		{name: "multiline closes at EOF", input: "FOO=\"\"\"a\nb\"\"\"", want: map[string]string{"FOO": "a\nb"}},
		{name: "multiline CRLF", input: "FOO=\"\"\"a\r\nb\"\"\"\r\nBAR=value", want: map[string]string{"FOO": "a\nb", "BAR": "value"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			secrets, err := parseSecrets(strings.NewReader(tt.input))
			require.NoError(t, err)
			assert.Equal(t, tt.want, secrets)
		})
	}
}

func TestParseSecretsMalformedEntryDiscardsPartialResults(t *testing.T) {
	secrets, err := parseSecrets(strings.NewReader("GOOD=value\nnot-an-assignment\nAFTER=value"))
	require.ErrorContains(t, err, "NAME=VALUE")
	assert.Nil(t, secrets)
}

func TestParseSecretsScannerBoundary(t *testing.T) {
	for _, tt := range []struct {
		name      string
		lineBytes int
		ending    string
		tooLong   bool
	}{
		{name: "last accepted line at EOF", lineBytes: bufio.MaxScanTokenSize - 1},
		{name: "last accepted line with LF", lineBytes: bufio.MaxScanTokenSize - 1, ending: "\n"},
		{name: "last accepted line with CRLF", lineBytes: bufio.MaxScanTokenSize - 2, ending: "\r\n"},
		{name: "limit at EOF", lineBytes: bufio.MaxScanTokenSize, tooLong: true},
		{name: "limit with LF", lineBytes: bufio.MaxScanTokenSize, ending: "\n", tooLong: true},
		{name: "limit with CRLF", lineBytes: bufio.MaxScanTokenSize, ending: "\r\n", tooLong: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			value := strings.Repeat("x", tt.lineBytes-len("FOO="))
			secrets, err := parseSecrets(strings.NewReader("FOO=" + value + tt.ending))
			if tt.tooLong {
				require.ErrorIs(t, err, bufio.ErrTooLong)
				assert.Nil(t, secrets)

				return
			}
			require.NoError(t, err)
			assert.Equal(t, map[string]string{"FOO": value}, secrets)
		})
	}
}

// failingSecretsReader exercises both permitted io.Reader error patterns:
// returning data with an error, or reporting the error on the next read.
type failingSecretsReader struct {
	data     string
	err      error
	together bool
}

func (r *failingSecretsReader) Read(p []byte) (int, error) {
	if len(r.data) == 0 {
		return 0, r.err
	}
	n := copy(p, r.data)
	r.data = r.data[n:]
	if r.together && len(r.data) == 0 {
		return n, r.err
	}

	return n, nil
}

func TestParseSecretsReaderFailure(t *testing.T) {
	readErr := errors.New("input stream failed")
	for _, tt := range []struct {
		name string
		data string
	}{
		{name: "before input"},
		{name: "after complete line", data: "GOOD=value\n"},
		{name: "after final line without newline", data: "GOOD=value"},
		{name: "during multiline", data: "GOOD=value\nKEY=\"\"\"partial\n"},
	} {
		for _, together := range []bool{false, true} {
			mode := "next read"
			if together {
				mode = "same read"
			}
			t.Run(tt.name+"/"+mode, func(t *testing.T) {
				secrets, err := parseSecrets(&failingSecretsReader{data: tt.data, err: readErr, together: together})
				require.ErrorIs(t, err, readErr)
				assert.ErrorContains(t, err, "failed to read secrets")
				assert.Nil(t, secrets)
			})
		}
	}
}

func FuzzParseSecrets(f *testing.F) {
	for _, seed := range []string{"", "FOO=", "FOO=\"", "FOO='", "FOO=\"\"", "FOO=\"\"\"\"\"\"", "GOOD=value\nBAD=\"\"\"partial", "FOO=\"\"\"a\nb\"\"\"", "FOO=\xff", "FOO=bar\r\n", "not-an-assignment"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, input string) {
		secrets, err := parseSecrets(strings.NewReader(input))
		if err != nil {
			assert.Nil(t, secrets)
		} else {
			assert.NotNil(t, secrets)
		}
	})
}
