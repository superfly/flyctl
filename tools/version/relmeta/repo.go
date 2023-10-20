package relmeta

import (
	"fmt"
	"os"
	"os/exec"
	"slices"
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
	// running in github actions, trigger event is a PR
	if ref := os.Getenv("GITHUB_HEAD_REF"); ref != "" {
		return ref, nil
	}
	// running in github actions, trigger event is a branch push
	if os.Getenv("GITHUB_REF_TYPE") == "branch" {
		if ref := os.Getenv("GITHUB_REF_NAME"); ref != "" {
			return ref, nil
		}
	}

	if isDetached, err := r.isDetachedHead(); err != nil {
		return "", errors.Wrap(err, "failed to check if git repo is detached")
	} else if isDetached {
		ref, err := r.gitRef()
		if err != nil {
			return "", errors.Wrap(err, "failed to get current git ref")
		}
		if branch, err := r.branchFromRef(ref); err != nil {
			return "", errors.Wrap(err, "failed to get branch from ref")
		} else {
			return branch, nil
		}
	}

	// find the branch from Git
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

func (r *gitRepo) isDetachedHead() (bool, error) {
	out, err := r.runGit("rev-parse", "--abbrev-ref", "--symbolic-full-name", "HEAD")
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(out) == "HEAD", nil
}

func (r *gitRepo) branchFromRef(ref string) (string, error) {
	out, err := r.runGit("branch", "-r", "--contains", ref)
	if err != nil {
		return "", errors.Wrap(err, "failed to get branches containing ref")
	}

	var branches []string
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "origin/") {
			branches = append(branches, strings.TrimPrefix(line, "origin/"))
		}
	}

	// prefer master/main branch if it contains this ref
	if slices.Contains(branches, "master") {
		return "master", nil
	}
	if slices.Contains(branches, "main") {
		return "main", nil
	}

	// otherwise, return the first branch that contains this ref
	if len(branches) > 0 {
		return branches[0], nil
	}

	return "", fmt.Errorf("no branch found containing ref \"%s\"", ref)
}

func channelFromRef(ref string) (string, error) {
	// return "pr<num>" if the ref is a PR
	if strings.HasPrefix(ref, "refs/pull/") {
		num, err := prNumber(ref)
		if err != nil {
			return "", errors.Wrapf(err, "failed to get PR number from ref %q", ref)
		}
		return fmt.Sprintf("pr%d", num), nil
	}

	// return the version's channel if ref is a tag
	if strings.HasPrefix(ref, "refs/tags/v") {
		ver, err := version.Parse(strings.TrimPrefix(ref, "refs/tags/v"))
		if err != nil {
			return "", errors.Wrapf(err, "failed to parse version from ref %q", ref)
		}
		return version.ChannelFromCalverOrSemver(ver), nil
	}

	// return the branch name if ref is a branch
	if strings.HasPrefix(ref, "refs/heads/") {
		return strings.TrimPrefix(ref, "refs/heads/"), nil
	}

	return "", fmt.Errorf("unable to determine channel from ref: %q", ref)
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
