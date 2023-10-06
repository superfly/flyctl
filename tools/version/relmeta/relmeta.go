package relmeta

import (
	"fmt"
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

func NextVersion(dir string) (*version.Version, error) {
	repo := newGitRepo(dir)
	ref, err := repo.gitRef()
	if err != nil {
		return nil, err
	}
	channel, err := channelFromRef(ref)
	if err != nil {
		return nil, err
	}

	tag, err := repo.previousTagOnChannel(channel)
	if err != nil {
		return nil, err
	}

	ver, err := version.Parse(tag)
	if err != nil {
		return nil, err
	}

	nextVer := ver.Increment(time.Now())
	return &nextVer, nil
}

func GenerateReleaseMeta(dir string, stillOnSemver bool) (release.Meta, error) {
	repo := newGitRepo(dir)

	output := release.Meta{}

	branch, err := repo.gitBranch()
	if err != nil {
		return output, err
	}
	output.Branch = branch

	commit, err := repo.gitCommitSHA()
	if err != nil {
		return output, err
	}
	output.Commit = commit

	ref, err := repo.gitRef()
	if err != nil {
		return output, err
	}
	output.Ref = ref

	commitTime, err := repo.gitCommitTime(commit)
	if err != nil {
		return output, err
	}
	output.CommitTime = commitTime

	dirty, err := repo.gitDirty()
	if err != nil {
		return output, err
	}
	output.Dirty = dirty

	channel, err := channelFromRef(ref)
	if err != nil {
		return output, err
	}
	output.Channel = channel

	currentTag, err := repo.currentTag(commit)
	fmt.Println("currentTag", currentTag)
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
