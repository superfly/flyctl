package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/superfly/flyctl/internal/version"
)

// numbers are based on commit time
// track is based on branch name, or PR number

// GITHUB_ACTIONS=true
// GITHUB_SHA=e27c15c6853de3735da04ad59d963af83f42aee2
// GITHUB_WORKFLOW_REF=michaeldwan/actions-test/.github/workflows/build.yml@refs/heads/main
// GITHUB_REF=refs/heads/main
// GITHUB_EVENT_NAME=workflow_dispatch
// GITHUB_RUN_ID=6042463302
// GITHUB_WORKFLOW_SHA=e27c15c6853de3735da04ad59d963af83f42aee2
// GITHUB_REF_NAME=main
// GITHUB_JOB=test_context
// GITHUB_HEAD_REF=
// GITHUB_ACTION_REF=
// GITHUB_BASE_REF=
// GITHUB_REPOSITORY=michaeldwan/actions-test
// GITHUB_REF=refs/pull/1/merge

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

func listTags(fn func(tag string) bool) error {
	if mockTags != nil {
		for _, tag := range mockTags {
			if !fn(tag) {
				break
			}
		}
		return nil
	}

	cmd := exec.Command("git", "tag", "-l", "--sort=version:refname", "v*")
	outPipe, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	errPipe, err := cmd.StderrPipe()
	if err != nil {
		return err
	}
	go io.Copy(os.Stderr, errPipe)

	if err := cmd.Start(); err != nil {
		return err
	}
	scanner := bufio.NewScanner(outPipe)

	for scanner.Scan() {
		if !fn(scanner.Text()) {
			break
		}
	}

	if err := cmd.Wait(); err != nil {
		return err
	}

	return nil
}

func latestVersion(track string) (*version.Version, error) {
	var latest *version.Version
	err := listTags(func(tag string) bool {
		v, err := version.Parse(tag)
		if err != nil {
			return false
		}

		if v.Track == track {
			latest = &v
			return false
		}

		return true
	})

	if err != nil {
		return nil, err
	}

	return latest, nil
}

func trackFromRef(ref string) (string, error) {
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

func taggedVersionsForTrack(track string) ([]version.Version, error) {
	versions := []version.Version{}

	err := listTags(func(tag string) bool {
		v, err := version.Parse(tag)
		if err != nil {
			return false
		}

		if v.Track == track {
			versions = append(versions, v)
		}
		return true
	})

	if err != nil {
		return nil, err
	}

	slices.SortFunc(versions, version.Compare)
	slices.Reverse(versions)

	return versions, nil
}
