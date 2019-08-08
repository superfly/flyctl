package docker

import (
	"fmt"
	"os"
	"path"

	"github.com/superfly/flyctl/flyctl"
	"github.com/superfly/flyctl/helpers"
	git "gopkg.in/src-d/go-git.v4"
)

const originUrl = "https://github.com/superfly/builders"

type BuilderRepo struct {
	path string
	repo *git.Repository
}

func NewBuilderRepo() (*BuilderRepo, error) {
	dir, err := buildersDir()
	if err != nil {
		return nil, err
	}

	repo, err := git.PlainOpen(dir)
	if err == git.ErrRepositoryNotExists {
		repo, err = git.PlainClone(dir, false, &git.CloneOptions{
			Depth:    1,
			URL:      originUrl,
			Progress: os.Stdout,
		})

	}
	if err != nil {
		return nil, err
	}

	out := &BuilderRepo{
		path: dir,
		repo: repo,
	}

	return out, nil
}

func buildersDir() (string, error) {
	configDir, err := flyctl.ConfigDir()
	if err != nil {
		return "", err
	}
	return path.Join(configDir, "builders"), nil
}

func (b *BuilderRepo) GetBuilder(name string) (builder, error) {
	path := path.Join(b.path, name)
	if helpers.DirectoryExists(path) {
		return builder{name, path}, nil
	}
	return builder{}, fmt.Errorf("Builder '%s' not found", name)
}

func (b *BuilderRepo) Sync() error {
	w, err := b.repo.Worktree()
	if err != nil {
		return err
	}

	err = w.Pull(&git.PullOptions{Force: true, RemoteName: "origin"})
	if err != nil && err != git.NoErrAlreadyUpToDate {
		return err
	}

	return nil
}

func (b *BuilderRepo) CurrentVersion() (string, error) {
	ref, err := b.repo.Head()
	if err != nil {
		return "", err
	}

	return ref.Hash().String(), nil
}

func (b *BuilderRepo) Destroy() error {
	return os.RemoveAll(b.path)
}

type builder struct {
	name string
	path string
}
