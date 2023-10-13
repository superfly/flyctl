package relmeta

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/superfly/flyctl/internal/version"
)

type gitError struct {
	message string
}

func (e gitError) Error() string {
	return "git error: " + e.message
}

type gitRepo struct {
	wd  string
	env []string
}

func newGitRepo(dir string) *gitRepo {
	return &gitRepo{wd: dir}
}

func (r *gitRepo) runGit(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = r.wd
	cmd.Env = r.env
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", gitError{message: string(out)}
	}
	return strings.TrimSpace(string(out)), nil
}

func (r *gitRepo) gitCommitTime(ref string) (time.Time, error) {
	out, err := r.runGit("show", "-s", "--format=%ct", ref)
	if err != nil {
		return time.Time{}, err
	}
	i, err := strconv.ParseInt(strings.TrimSpace(out), 10, 64)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid commit time - expected \"%s\" to be a unix timestamp", out)
	}
	return time.Unix(i, 0).UTC(), nil
}

func (r *gitRepo) gitBranch() (string, error) {
	if ref := os.Getenv("GITHUB_HEAD_REF"); ref != "" {
		return ref, nil
	}
	if ref := os.Getenv("GITHUB_REF_NAME"); ref != "" {
		return ref, nil
	}

	output, err := r.runGit("rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return "", errors.Wrap(err, "failed to get current git branch")
	}

	return strings.TrimSpace(output), nil
}

func (r *gitRepo) gitRef() (string, error) {
	ref := os.Getenv("GITHUB_REF")
	if ref != "" {
		return ref, nil
	}

	output, err := r.runGit("rev-parse", "--symbolic-full-name", "HEAD")
	if err != nil {
		return "", errors.Wrap(err, "failed to get current git ref")
	}

	return strings.TrimSpace(output), nil
}

func (r *gitRepo) gitCommitSHA() (string, error) {
	output, err := r.runGit("rev-parse", "HEAD")
	if err != nil {
		return "", errors.Wrap(err, "failed to get current git sha")
	}

	return strings.TrimSpace(output), nil
}

func (r *gitRepo) gitDirty() (bool, error) {
	output, err := r.runGit("status", "--porcelain")
	if err != nil {
		return false, errors.Wrap(err, "failed to get git status")
	}

	return strings.TrimSpace(output) != "", nil
}

func (r *gitRepo) RefreshTags() error {
	originName := "origin"
	_, err := r.runGit("fetch", "--tags", originName)
	return err
}

func (r *gitRepo) currentTag(sha string) (string, error) {
	out, err := r.runGit("tag", "--points-at", sha, "v*")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

func (r *gitRepo) previousTagOnChannel(channel string, semverOnly bool) (string, error) {
	out, err := r.runGit("tag", "-l", "--sort=-version:refname", "v*")
	if err != nil {
		return "", err
	}
	tags := strings.Split(out, "\n")

	// var latest *version.Version
	var latestTag string
	for _, tag := range tags {
		v, err := version.Parse(tag)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			continue
		}

		// semver stable doesn't have a track. check that it's empty. remove this once the calver migration is done
		if semverOnly && !version.IsCalVer(v) && channel == "stable" && v.Channel == "" {
			// latest = &v
			latestTag = tag
			break
		}

		if v.Channel == channel {
			// latest = &v
			latestTag = tag
			break
		}
	}

	return latestTag, nil
}

func (r *gitRepo) previousTag(currentTag string) (string, error) {
	// git describe --abbrev=0 --tags --exclude="$(git describe --abbrev=0 --tags --match 'v*')" --match 'v*'
	out, err := r.runGit("describe", "--abbrev=0", "--tags", "--exclude="+currentTag, "--match=v*")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

func channelFromRef(ref string) (string, error) {
	// track is always "dev" unless built on CI
	// if !isCI() {
	// 	return "dev", nil
	// }

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

	if headRef := os.Getenv("GITHUB_HEAD_REF"); headRef != "" {
		return headRef, nil
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
