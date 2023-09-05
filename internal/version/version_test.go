package version

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestToString(t *testing.T) {
	tests := []struct {
		v        Version
		expected string
	}{
		// v0.1.87
		{Version{0, 1, 87, 1, "stable"}, "0.1.87"},
		{Version{0, 1, 87, 0, ""}, "0.1.87"},
		// v0.1.85-pre-1
		{Version{0, 1, 85, 1, "pre"}, "0.1.85-pre-1"},
		{Version{2023, 9, 5, 0, ""}, "2023.9.5"}, // this one is dubious
		{Version{2023, 9, 5, 1, "stable"}, "2023.9.5-stable.1"},
		{Version{2023, 9, 5, 2, "stable"}, "2023.9.5-stable.2"},
		{Version{2023, 9, 5, 1, "pr123"}, "2023.9.5-pr123.1"},
		{Version{2023, 9, 5, 1, "my/feature/branch"}, "2023.9.5-my-feature-branch.1"},
	}

	for _, test := range tests {
		t.Run(fmt.Sprintf("%v", test.v), func(t *testing.T) {
			assert.Equal(t, test.expected, test.v.String())
		})
	}
}

func TestParse(t *testing.T) {
	cases := []struct {
		in   string
		want Version
		err  error
	}{
		{"1.2.3", Version{1, 2, 3, 1, "stable"}, nil},
		{"1.2.3", Version{1, 2, 3, 1, "stable"}, nil},
		{"1.2.3-pre.1", Version{1, 2, 3, 1, "pre"}, nil},
		{"0.1.123-stable.1", Version{0, 1, 123, 1, "stable"}, nil},
		{"0.1.123-stable.123", Version{0, 1, 123, 123, "stable"}, nil},
		{"0.1.123-pre", Version{0, 1, 123, 1, "pre"}, nil},
		{"0.1.123-pre-1", Version{0, 1, 123, 1, "pre"}, nil},
		{"0.1.123-pre-2", Version{0, 1, 123, 2, "pre"}, nil},
		// {"2023.08.16", Version{2023, 8, 16, 0, ""}, nil},
		{"2023.8.16", Version{2023, 8, 16, 1, "stable"}, nil},
		{"2023.8.16-stable", Version{2023, 8, 16, 1, "stable"}, nil},
		// {"2023.8.16-1", Version{2023, 8, 16, 1, ""}, nil},
		{"2023.8.16-stable.2", Version{2023, 8, 16, 2, "stable"}, nil},
	}

	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			actual, err := Parse(c.in)
			if c.err != nil {
				assert.EqualError(t, err, c.err.Error())
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, c.want, actual)
		})
		// assert.EqualError(t, err, c.err.Error())
	}
}

func TestParseVPrefix(t *testing.T) {
	v1, err1 := Parse("0.1.2")
	v2, err2 := Parse("v0.1.2")
	assert.NoError(t, err1)
	assert.NoError(t, err2)
	assert.Equal(t, Version{0, 1, 2, 0, ""}, v1)
	assert.Equal(t, v1, v2)
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
		assert.Equal(t, test.want, Compare(a, b), "eq(%q, %q) should be %d", test.a, test.b, test.want)
	}
}

func TestSort(t *testing.T) {
	t.Fail()
}
