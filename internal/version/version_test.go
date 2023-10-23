package version

import (
	"encoding/json"
	"math/rand"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/exp/slices"
)

func TestEncode(t *testing.T) {
	cases := map[string]Version{
		"0.0.1":                              {0, 0, 1, 0, "", ""},
		"0.0.10":                             {0, 0, 10, 0, "", ""},
		"0.0.138":                            {0, 0, 138, 0, "", ""},
		"0.0.138-beta-1":                     {0, 0, 138, 1, "beta", ""},
		"0.0.138-beta-10":                    {0, 0, 138, 10, "beta", ""},
		"0.0.218-pre-1":                      {0, 0, 218, 1, "pre", ""},
		"0.0.218-pre-10":                     {0, 0, 218, 10, "pre", ""},
		"0.1.0":                              {0, 1, 0, 0, "", ""},
		"0.1.1":                              {0, 1, 1, 0, "", ""},
		"0.1.10":                             {0, 1, 10, 0, "", ""},
		"0.1.44":                             {0, 1, 44, 0, "", ""},
		"0.1.44-pre-2":                       {0, 1, 44, 2, "pre", ""},
		"0.0.269-dev-tqbf-tcp-proxy-48b8696": {0, 0, 269, 0, "dev-tqbf-tcp-proxy-48b8696", ""},

		"2023.1.1":                     {2023, 1, 1, 0, "", ""},
		"2023.1.12":                    {2023, 1, 12, 0, "", ""},
		"2023.12.1":                    {2023, 12, 1, 0, "", ""},
		"2023.12.12":                   {2023, 12, 12, 0, "", ""},
		"2023.8.16":                    {2023, 8, 16, 0, "", ""},
		"2023.8.16-stable":             {2023, 8, 16, 0, "stable", ""},
		"2023.8.16-stable.1":           {2023, 8, 16, 1, "stable", ""},
		"2023.8.16-stable.12":          {2023, 8, 16, 12, "stable", ""},
		"2023.8.16-stable.123":         {2023, 8, 16, 123, "stable", ""},
		"2023.8.16-pr1234.1":           {2023, 8, 16, 1, "pr1234", ""},
		"2023.8.16-pr1234.12":          {2023, 8, 16, 12, "pr1234", ""},
		"2023.8.16-pr1234.123":         {2023, 8, 16, 123, "pr1234", ""},
		"2023.9.5-my-feature-branch.1": {2023, 9, 5, 1, "my-feature-branch", ""},

		"0.0.0-dev":                  {0, 0, 0, 0, "dev", ""},
		"0.0.0-dev.1694038019":       {0, 0, 0, 1694038019, "dev", ""},
		"0.0.0-dev+gitsha":           {0, 0, 0, 0, "dev", "gitsha"},
		"0.0.0-dev+gitsha-dirty":     {0, 0, 0, 0, "dev", "gitsha-dirty"},
		"0.0.0-dev+123-gitsha":       {0, 0, 0, 0, "dev", "123-gitsha"},
		"0.0.0-dev+123-gitsha-dirty": {0, 0, 0, 0, "dev", "123-gitsha-dirty"},

		"2023.10.5-some-branch+gitsha":           {2023, 10, 5, 0, "some-branch", "gitsha"},
		"2023.10.5-some-branch+gitsha-dirty":     {2023, 10, 5, 0, "some-branch", "gitsha-dirty"},
		"2023.10.5-some-branch+123-gitsha":       {2023, 10, 5, 0, "some-branch", "123-gitsha"},
		"2023.10.5-some-branch+123-gitsha-dirty": {2023, 10, 5, 0, "some-branch", "123-gitsha-dirty"},

		"2023.10.23-dependabot/go_modules/nhooyr.io/websocket-1.8.9.1698092436": {2023, 10, 23, 1698092436, "dependabot/go_modules/nhooyr.io/websocket-1.8.9", ""},
	}

	for vString, vStruct := range cases {
		t.Run(vString, func(t *testing.T) {
			t.Run("Parse", func(t *testing.T) {
				actual, err := Parse(vString)
				assert.NoError(t, err)
				assert.Equal(t, vStruct, actual)
			})

			t.Run("ToString", func(t *testing.T) {
				assert.Equal(t, vString, vStruct.String())
			})
		})
	}
}

func TestParseVPrefix(t *testing.T) {
	v1, err1 := Parse("0.1.2")
	v2, err2 := Parse("v0.1.2")
	expected := Version{0, 1, 2, 0, "", ""}
	assert.NoError(t, err1)
	assert.NoError(t, err2)
	assert.Equal(t, expected, v1)
	assert.Equal(t, expected, v2)
}

func TestEquality(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"1.2.3", "1.2.3", 0},
		{"1.2.3-pre-1", "1.2.3-pre-1", 0},
		{"0.1.79", "0.1.78", 1},
		{"0.1.78", "0.1.79", -1},

		{"0.1.78-pre-1", "0.1.79", -1},
		{"0.1.78-stable-1", "0.1.79", -1},
		{"0.1.78-pre-1", "0.1.78", -1},

		{"2023.9.6", "2022.9.6", 1},
		{"2023.9.6", "2023.8.6", 1},
		{"2023.9.6", "2023.9.5", 1},

		{"2023.9.6-stable.2", "2023.9.6-stable.1", 1},
		{"2023.9.6-stable.123", "2023.9.6-stable.12", 1},
		{"2023.9.6", "2023.9.6-stable.1", 1},
	}

	for _, test := range tests {
		a, err := Parse(test.a)
		assert.NoError(t, err)
		b, err := Parse(test.b)
		assert.NoError(t, err)
		assert.Equal(t, test.want, Compare(a, b), "eq(%q, %q) should be %d", test.a, test.b, test.want)
		assert.Equal(t, test.want*-1, Compare(b, a), "eq(%q, %q) should be %d", test.b, test.a, test.want*-1)
	}
}

func TestSort(t *testing.T) {
	expectedVersions := []string{
		"0.0.1",
		"0.0.10",
		"0.0.138-beta-1",
		"0.0.138-beta-10",
		"0.0.138",
		"0.0.218-pre-1",
		"0.0.218-pre-10",
		"0.0.218",
		"0.0.269-dev-tqbf-tcp-proxy-48b8696",
		"0.1.0",
		"0.1.1",
		"0.1.10",
		"0.1.44-pre-2",
		"0.1.44",
		"2023.1.1",
		"2023.1.12",
		"2023.8.16-pr1234.1",
		"2023.8.16-pr1234.12",
		"2023.8.16-pr1234.123",
		"2023.8.16-stable",
		"2023.8.16-stable.1",
		"2023.8.16-stable.12",
		"2023.8.16-stable.123",
		"2023.8.16",
		"2023.9.5-my-feature-branch.1",
		"2023.12.1",
		"2023.12.12",
	}

	versions := make([]Version, len(expectedVersions))
	for idx, vStr := range expectedVersions {
		v, err := Parse(vStr)
		assert.NoError(t, err)
		versions[idx] = v
	}
	rand.Shuffle(len(versions), func(i, j int) {
		versions[i], versions[j] = versions[j], versions[i]
	})

	slices.SortFunc(versions, Compare)

	sortedVersions := []string{}
	for _, v := range versions {
		sortedVersions = append(sortedVersions, v.String())
	}

	assert.Equal(t, expectedVersions, sortedVersions)
}

func TestSignificantlyBehind(t *testing.T) {
	tests := []struct {
		current, latest string
		want            bool
	}{
		{"0.0.123", "0.1.123", true},
		{"0.1.123", "0.1.123", false},
		{"0.1.123", "0.1.128", false},
		{"0.1.123", "0.1.129", true},

		{"2023.8.1", "2023.8.2", false},
		{"2023.8.1", "2023.8.29", true},
		{"2023.8.1", "2023.9.1", true},
	}

	for _, test := range tests {
		currentVer, err := Parse(test.current)
		assert.NoError(t, err)
		latestVer, err := Parse(test.latest)
		assert.NoError(t, err)
		assert.Equal(t, test.want, currentVer.SignificantlyBehind(latestVer), "%q<>%q", test.current, test.latest)
	}
}

func TestIsCalver(t *testing.T) {
	tests := map[string]bool{
		"1.2.3":         false,
		"0.0.0":         false,
		"2023.8.16":     true,
		"2023.8.16-pre": true,
		"0.1.87":        false,
		"0.0.503":       false,
		"0.0.503-pre-3": false,
	}

	for input, expected := range tests {
		t.Run(input, func(t *testing.T) {
			v, err := Parse(input)
			assert.NoError(t, err)
			assert.Equal(t, expected, IsCalVer(v))
		})
	}
}

func TestIncrement(t *testing.T) {
	tests := []struct {
		current, next, commitDate string
	}{
		{"2023.8.25-stable.1", "2023.8.25-stable.2", "2023-08-25T00:00:00Z"},
		{"2023.8.25-stable.2", "2023.8.25-stable.3", "2023-08-25T00:00:00Z"},
		{"2023.8.25-pr1234.3", "2023.8.25-pr1234.4", "2023-08-25T00:00:00Z"},
		{"2023.8.25-stable.3", "2023.8.26-stable.1", "2023-08-26T00:00:00Z"},
		{"2023.9.7-flypkgs.1", "2023.9.7-flypkgs.2", "2023-09-07T00:00:00Z"},
		{"2023.9.7-flypkgs.1", "2023.9.7-flypkgs.2", "2023-09-07T13:31:01Z"},
	}

	for _, test := range tests {
		t.Run(test.current+"<>"+test.commitDate, func(t *testing.T) {
			currentVer, err := Parse(test.current)
			assert.NoError(t, err)
			commitDate, err := time.Parse(time.RFC3339, test.commitDate)
			assert.NoError(t, err)
			nextVer := currentVer.Increment(commitDate)
			assert.Equal(t, test.next, nextVer.String())
		})
	}
}

func TestJSONMarshalling(t *testing.T) {
	v1 := New(time.Date(2023, 8, 16, 0, 0, 0, 0, time.UTC), "stable", 1)
	data, err := json.Marshal(v1)
	require.NoError(t, err)
	v2 := Version{}
	err = json.Unmarshal(data, &v2)
	require.NoError(t, err)
	assert.Equal(t, v1, v2)
	assert.True(t, v1.Equal(v2))
}

func TestNestedJSONMarshalling(t *testing.T) {
	a := struct{ V Version }{
		New(time.Date(2023, 8, 16, 0, 0, 0, 0, time.UTC), "stable", 1),
	}

	data, err := json.Marshal(a)
	require.NoError(t, err)

	b := struct{ V Version }{}
	err = json.Unmarshal(data, &b)
	require.NoError(t, err)
	assert.Equal(t, a, b)
	// assert.True(t, v1.Equal(v2))
}
