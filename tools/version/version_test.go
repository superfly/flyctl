package main

import (
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/superfly/flyctl/internal/version"
)

func TestGitRefFromGitHubEnvVar(t *testing.T) {
	t.Setenv("GITHUB_REF", "refs/pull/123/merge")
	ref, err := gitRef()
	assert.NoError(t, err)
	assert.Equal(t, "refs/pull/123/merge", ref)
}

func TestGitRefFromGit(t *testing.T) {
	t.Setenv("GITHUB_REF", "")

	expectedBranch, err := runGit("branch", "--show-current")
	assert.NoError(t, err, "failed to get current branch for expected result")

	ref, err := gitRef()
	assert.NoError(t, err)
	assert.Equal(t, "refs/heads/"+expectedBranch, ref)
}

func TestCurrentCommitFromEnvVar(t *testing.T) {
	t.Setenv("GITHUB_SHA", "1234567890abcdef")
	commit, err := gitCommitSHA()
	assert.NoError(t, err)
	assert.Equal(t, "1234567890abcdef", commit)
}

func TestCurrentCommitSHAFromGit(t *testing.T) {
	t.Setenv("GITHUB_SHA", "")

	expectedSha, err := runGit("log", "-1", "--format=%H")
	assert.NoError(t, err, "failed to get current commit sha for expected result")

	commit, err := gitCommitSHA()
	assert.NoError(t, err)
	assert.Equal(t, expectedSha, commit)
}

func TestCommitTimeFromGit(t *testing.T) {
	t.Setenv("GITHUB_SHA", "")

	// use first commit for stable commit https://github.com/superfly/flyctl/commit/aa1d55f7
	commitTime, err := gitCommitTime("aa1d55f7")
	assert.NoError(t, err)
	assert.Equal(t, commitTime, time.Date(2019, 7, 26, 18, 3, 32, 0, time.UTC))
}

func TestIsStableBranch(t *testing.T) {
	tests := map[string]bool{
		"refs/heads/master":    true,
		"refs/heads/main":      true,
		"refs/heads/my-branch": false,
		"HEAD":                 false,
	}

	for ref, expected := range tests {
		t.Run(ref, func(t *testing.T) {
			stable := isRefStableBranch(ref)
			assert.Equal(t, expected, stable)
		})
	}
}

func TestTrackFromRef_CI(t *testing.T) {
	tests := []struct {
		ref           string
		expectedTrack string
		expectedErr   error
	}{
		{"refs/heads/master", "stable", nil},
		{"refs/heads/main", "stable", nil},
		{"refs/pull/5432/merge", "pr5432", nil},
		{"refs/pull/543/merge", "pr543", nil},
		{"refs/pull/54/merge", "pr54", nil},
		{"refs/pull/5/merge", "pr5", nil},
		{"refs/heads/my-branch", "my-branch", nil},
		{"refs/heads/feature/123", "feature/123", nil},
		{"refs/heads/feat/launch-v2/databases", "feat/launch-v2/databases", nil},
		{"refs/heads/fix/prompt-app-create-on-deploy", "fix/prompt-app-create-on-deploy", nil},
		{"refs/heads/dependabot/go_modules/github.com/vektah/gqlparser/v2-2.5.8", "dependabot/go_modules/github.com/vektah/gqlparser/v2-2.5.8", nil},
	}

	for _, test := range tests {
		t.Run(test.ref, func(t *testing.T) {
			t.Setenv("CI", "true")
			track, err := trackFromRef(test.ref)
			if test.expectedErr != nil {
				assert.EqualError(t, err, test.expectedErr.Error())
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, test.expectedTrack, track)
		})
	}
}

func TestTrackFromRef_Dev(t *testing.T) {
	tests := []string{
		"refs/heads/master",
		"refs/heads/main",
		"refs/pull/5432/merge",
		"refs/pull/543/merge",
		"refs/pull/54/merge",
		"refs/pull/5/merge",
		"refs/heads/my-branch",
		"refs/heads/feature/123",
		"refs/heads/feat/launch-v2/databases",
		"refs/heads/fix/prompt-app-create-on-deploy",
		"refs/heads/dependabot/go_modules/github.com/vektah/gqlparser/v2-2.5.8",
	}

	for _, test := range tests {
		t.Run(test, func(t *testing.T) {
			t.Setenv("CI", "")
			t.Setenv("GITHUB_ACTIONS", "")

			track, err := trackFromRef(test)
			assert.NoError(t, err)
			assert.Equal(t, "dev", track)
		})
	}
}

func TestLatestVersionForTrack(t *testing.T) {
	mockTags = []string{
		"v2023.9.5-stable.1",
		"v2023.9.5-pr123.3",
		"v2023.9.2-stable.2",
		"v2023.9.2-stable.1",
		"v2023.9.1-pr123.2",
		"v2023.8.1-pr123.1",
		"v0.1.87",
		"v0.1.85",
		"v0.1.85-pre-1",
	}

	tests := map[string]*version.Version{
		"stable":        {Major: 2023, Minor: 9, Patch: 5, Build: 1, Track: "stable"},
		"pr123":         {Major: 2023, Minor: 9, Patch: 5, Build: 3, Track: "pr123"},
		"missing-track": nil,
	}

	for track, expectedVersion := range tests {
		t.Run(track, func(t *testing.T) {
			latest, err := latestVersion(track)
			assert.NoError(t, err)
			assert.Equal(t, expectedVersion, latest)
		})
	}
}

func TestTaggedVersionsForTrack(t *testing.T) {
	mockTags = []string{
		"v2023.9.5-stable.1",
		"v2023.9.2-stable.1",
		"v2023.9.2-stable.2",
		"v2023.8.1-pr123.1",
		"v2023.9.1-pr123.2",
		"v2023.9.5-pr123.3",
		"v0.1.87",
		"v0.1.85-pre-1",
		"v0.1.85",
	}

	tests := []struct {
		track          string
		expectedResult []version.Version
	}{
		{"", []version.Version{
			{Major: 0, Minor: 1, Patch: 87, Build: 0, Track: ""},
			{Major: 0, Minor: 1, Patch: 85, Build: 0, Track: ""},
		}},
		{"stable", []version.Version{
			{Major: 2023, Minor: 9, Patch: 5, Build: 1, Track: "stable"},
			{Major: 2023, Minor: 9, Patch: 2, Build: 2, Track: "stable"},
			{Major: 2023, Minor: 9, Patch: 2, Build: 1, Track: "stable"},
		}},
		{"pr123", []version.Version{
			{Major: 2023, Minor: 9, Patch: 5, Build: 3, Track: "pr123"},
			{Major: 2023, Minor: 9, Patch: 1, Build: 2, Track: "pr123"},
			{Major: 2023, Minor: 8, Patch: 1, Build: 1, Track: "pr123"},
		}},
		{"pre", []version.Version{
			{Major: 0, Minor: 1, Patch: 85, Build: 1, Track: "pre"},
		}},
	}

	for _, test := range tests {
		t.Run(test.track, func(t *testing.T) {
			versions, err := taggedVersionsForTrack(test.track)
			assert.NoError(t, err)
			assert.Equal(t, test.expectedResult, versions)
		})
	}
}

// parseVersions converts the input into a slice of version.Version structs
// sorted in descending order. This matches the output of taggedVersionsForTrack
func parseVersions(versions ...string) (out []version.Version) {
	for _, v := range versions {
		parsed, err := version.Parse(v)
		if err != nil {
			panic(err)
		}
		out = append(out, parsed)
	}

	slices.SortFunc(out, version.Compare)
	slices.Reverse(out)

	return
}

func parseCommitDate(commitDate string) time.Time {
	t, err := time.Parse(time.DateOnly, commitDate)
	if err != nil {
		panic(err)
	}
	return t
}

func TestBuildNumber_IncrementsBuildsOnSameDate(t *testing.T) {
	mockTags = []string{
		"v2023.9.5-stable.3",
		"v2023.9.5-stable.2",
		"v2023.9.5-stable.1",
		"v2023.9.3-stable.2",
		"v2023.9.3-stable.1",
		"v2023.9.1-stable.1",
	}

	buildNum, err := nextBuildNumber("stable", parseCommitDate("2023-09-05"))
	assert.NoError(t, err)
	assert.Equal(t, 4, buildNum)
}

func TestBuildNumber_ResetsOnNewDate(t *testing.T) {
	mockTags = []string{
		"v2023.9.5-stable.3",
		"v2023.9.5-stable.2",
		"v2023.9.5-stable.1",
		"v2023.9.3-stable.2",
		"v2023.9.3-stable.1",
		"v2023.9.1-stable.1",
	}

	buildNum, err := nextBuildNumber("stable", parseCommitDate("2023-09-06"))
	assert.NoError(t, err)
	assert.Equal(t, 1, buildNum)
}
