package main

import (
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/superfly/flyctl/internal/version"
)

func gitCommitTime(ref string) (time.Time, error) {
	out, err := runGit("show", "-s", "--format=%ct", ref)
	if err != nil {
		return time.Time{}, err
	}
	i, err := strconv.ParseInt(strings.TrimSpace(string(out)), 10, 64)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid commit time - expected \"%s\" to be a unix timestamp", string(out))
	}
	return time.Unix(i, 0).UTC(), nil
}

func gitRef() (string, error) {
	ref := os.Getenv("GITHUB_REF")
	if ref != "" {
		return ref, nil
	}

	output, err := runGit("rev-parse", "--symbolic-full-name", "HEAD")
	if err != nil {
		return "", errors.Wrap(err, "failed to get current git ref")
	}

	return strings.TrimSpace(output), nil
}

func gitCommitSHA() (string, error) {
	sha := os.Getenv("GITHUB_SHA")
	if sha != "" {
		return sha, nil
	}

	output, err := runGit("rev-parse", "HEAD")
	if err != nil {
		return "", errors.Wrap(err, "failed to get current git sha")
	}

	return strings.TrimSpace(output), nil
}

func refreshTags() error {
	originName := "origin"
	_, err := runGit("fetch", "--tags", originName)
	return err
}

var mockTags []string

func listVersionTags() ([]string, error) {
	if mockTags != nil {
		return mockTags, nil
	}

	out, err := runGit("tag", "-l", "--sort=-version:refname", "v*")
	if err != nil {
		return nil, err
	}
	return strings.Split(string(out), "\n"), nil
}

func latestVersion(track string, semverOnly bool) (*version.Version, error) {
	tags, err := listVersionTags()
	if err != nil {
		return nil, err
	}

	var latest *version.Version
	for _, tag := range tags {
		v, err := version.Parse(tag)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			continue
		}

		// semver stable doesn't have a track. check that it's empty. remove this once the calver migration is done
		if semverOnly && !version.IsCalVer(v) && track == "stable" && v.Channel == "" {
			latest = &v
			break
		}

		if v.Channel == track {
			latest = &v
			break
		}
	}

	return latest, nil
}

func channelFromRef(ref string) (string, error) {
	// track is always "dev" unless built on CI
	if !isCI() {
		return "dev", nil
	}

	if strings.HasPrefix(ref, "refs/pull/") {
		num, err := prNumber(ref)
		if err != nil {
			return "", errors.Wrapf(err, "failed to get PR number from ref \"%s\"", ref)
		}
		return fmt.Sprintf("pr%d", num), nil
	}

	if isRefStableBranch(ref) {
		return "stable", nil
	}

	branch, err := branchFromRef(ref)
	if err != nil {
		return "", errors.Wrapf(err, "unable to select track from ref \"%s\"", ref)
	}

	return branch, nil
}

func branchFromRef(ref string) (string, error) {
	if strings.HasPrefix(ref, "refs/heads/") {
		return strings.TrimPrefix(ref, "refs/heads/"), nil
	}
	return "", fmt.Errorf("invalid branch ref \"%s\"", ref)
}

func isCI() bool {
	return os.Getenv("GITHUB_ACTIONS") == "true" || os.Getenv("CI") == "true"
}

func isRefStableBranch(ref string) bool {
	return ref == "refs/heads/master" || ref == "refs/heads/main"
}

func prNumber(ref string) (int, error) {
	parts := strings.Split(ref, "/")
	if len(parts) != 4 {
		return -1, fmt.Errorf("invalid PR ref \"%s\"", ref)
	}
	num, err := strconv.Atoi(parts[2])
	if err != nil {
		return -1, errors.Wrapf(err, "error parsing PR number from ref \"%s\"", ref)
	}
	return num, nil
}

func taggedVersionsForChannel(channel string) ([]version.Version, error) {
	versions := []version.Version{}

	tags, err := listVersionTags()
	if err != nil {
		return nil, err
	}
	for _, tag := range tags {
		if v, err := version.Parse(tag); err == nil {
			if v.Channel == channel {
				versions = append(versions, v)
			}
		} else {
			fmt.Fprintln(os.Stderr, err)
		}
	}

	slices.SortFunc(versions, version.Compare)
	slices.Reverse(versions)

	return versions, nil
}
