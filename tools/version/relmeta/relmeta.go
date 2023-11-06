package relmeta

import (
	"fmt"
	"os"
	"time"

	"github.com/superfly/flyctl/internal/release"
	"github.com/superfly/flyctl/internal/version"
)

func RefreshTags(dir string) error {
	repo := newGitRepo(dir)
	return repo.RefreshTags()
}

func CurrentVersion(dir string) (*version.Version, error) {
	meta, err := GenerateReleaseMeta(dir, true)
	if err != nil {
		return nil, err
	}

	return meta.Version, nil
}

func NextVersion(dir string, semverOnly bool) (version.Version, error) {
	repo := newGitRepo(dir)
	ref, err := repo.gitRef()
	if err != nil {
		return version.Version{}, err
	}
	fmt.Fprintln(os.Stderr, "ref:", ref)

	channel, err := channelFromRef(ref)
	if err != nil {
		return version.Version{}, err
	}
	fmt.Fprintln(os.Stderr, "channel:", channel)

	tag, err := repo.previousTagOnChannel(channel, semverOnly)
	if err != nil {
		return version.Version{}, err
	}
	if tag == "" {
		tag, err = repo.previousTagOnChannel("stable", semverOnly)
		if err != nil {
			return version.Version{}, err
		}
	}

	fmt.Fprintln(os.Stderr, "previous tag:", tag)

	if tag == "" {
		return version.New(time.Now(), channel, 1), nil
	}

	ver, err := version.Parse(tag)
	if err != nil {
		return version.Version{}, err
	}
	fmt.Fprintln(os.Stderr, "parsed version:", ver)

	if ver.Channel != channel {
		return version.New(time.Now(), channel, 1), nil
	}

	return ver.Increment(time.Now()), nil
}

func GenerateReleaseMeta(dir string, stillOnSemver bool) (release.Meta, error) {
	repo := newGitRepo(dir)

	output := release.Meta{}

	branch, err := repo.gitBranch()
	if err != nil {
		return output, err
	}
	output.Branch = branch
	fmt.Fprintln(os.Stderr, "branch:", branch)

	commit, err := repo.gitCommitSHA()
	if err != nil {
		return output, err
	}
	output.Commit = commit
	fmt.Fprintln(os.Stderr, "commit:", commit)

	ref, err := repo.gitRef()
	if err != nil {
		return output, err
	}
	output.Ref = ref
	fmt.Fprintln(os.Stderr, "ref:", ref)

	commitTime, err := repo.gitCommitTime(commit)
	if err != nil {
		return output, err
	}
	output.CommitTime = commitTime
	fmt.Fprintln(os.Stderr, "commitTime:", commitTime)

	dirty, err := repo.gitDirty()
	if err != nil {
		return output, err
	}
	output.Dirty = dirty
	fmt.Fprintln(os.Stderr, "dirty:", dirty)

	channel, err := channelFromRef(ref)
	if err != nil {
		return output, err
	}
	output.Channel = channel
	fmt.Fprintln(os.Stderr, "channel:", channel)

	currentTag, err := repo.currentTag(commit)
	if err != nil {
		return output, err
	}
	if currentTag != "" {
		output.Tag = currentTag

		currentVersion, err := version.Parse(currentTag)
		if err != nil {
			return output, err
		}
		output.Version = &currentVersion
	}

	previousTag, err := repo.previousTag(currentTag)
	if err != nil {
		return output, err
	}
	if previousTag != "" {
		output.PreviousTag = previousTag
		previousVersion, err := version.Parse(previousTag)
		if err != nil {
			return output, err
		}
		output.PreviousVersion = &previousVersion
	}

	return output, nil
}
