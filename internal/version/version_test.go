package version

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParse(t *testing.T) {
	cases := []struct {
		in   string
		want Version
		err  error
	}{
		{"1.2.3", Version{1, 2, 3, 0, ""}, nil},
		{"1.2.3", Version{1, 2, 3, 0, ""}, nil},
		{"1.2.3-pre.1", Version{1, 2, 3, 1, "pre"}, nil},
		{"0.1.123-stable.1", Version{0, 1, 123, 1, "stable"}, nil},
		{"0.1.123-stable.123", Version{0, 1, 123, 123, "stable"}, nil},
		{"0.1.123-pre", Version{0, 1, 123, 0, "pre"}, nil},
		// {"2023.08.16", Version{2023, 8, 16, 0, ""}, nil},
		{"2023.8.16", Version{2023, 8, 16, 0, ""}, nil},
		{"2023.8.16-stable", Version{2023, 8, 16, 0, "stable"}, nil},
		// {"2023.8.16-1", Version{2023, 8, 16, 1, ""}, nil},
		{"2023.8.16-stable.2", Version{2023, 8, 16, 2, "stable"}, nil},
	}

	for _, c := range cases {
		actual, err := Parse(c.in)
		assert.Equal(t, c.want, actual)
		assert.Equal(t, c.err, err)
		assert.Equal(t, c.in, actual.String())
		// assert.EqualError(t, err, c.err.Error())
	}
}

func TestParseVPrefix(t *testing.T) {
	v1, err1 := Parse("0.1.2")
	v2, err2 := Parse("v0.1.2")

	assert.Equal(t, v1, v2)
	assert.Equal(t, err1, err2)
}

func TestEquality(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"1.2.3", "1.2.3", 0},
		{"1.2.3-pre.1", "1.2.3-pre.1", 0},
		{"0.1.79", "0.1.78", 1},
		{"0.1.78", "0.1.79", -1},

		{"0.1.78-pre.1", "0.1.79", -1},
		{"0.1.78-stable.1", "0.1.79", -1},
		{"0.1.78-pre.1", "0.1.78", -1},
	}

	for _, test := range tests {
		a, err := Parse(test.a)
		assert.NoError(t, err)
		b, err := Parse(test.b)
		assert.NoError(t, err)
		assert.Equal(t, test.want, eq(a, b), "eq(%q, %q) should be %d", test.a, test.b, test.want)
	}

}
