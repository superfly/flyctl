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
}

func NewBuilderRepo() (*BuilderRepo, error) {
	dir, err := buildersDir()
	if err != nil {
		return nil, err
	}

	r := &BuilderRepo{
		path: dir,
	}

	return r, nil
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
	repo, err := git.PlainOpen(b.path)
	if err == git.ErrRepositoryNotExists {
		repo, err = b.initBuildersRepo()
	}

	if err != nil {
		return err
	}

	w, err := repo.Worktree()
	if err != nil {
		return err
	}

	err = w.Pull(&git.PullOptions{Force: true, RemoteName: "origin"})
	if err != nil && err != git.NoErrAlreadyUpToDate {
		return err
	}

	fmt.Printf("%+v\n", repo)

	ref, err := repo.Head()
	if err != nil {
		panic(err)
	}

	commit, err := repo.CommitObject(ref.Hash())

	fmt.Println(commit)

	return nil
}

func (b *BuilderRepo) Destroy() error {
	return os.RemoveAll(b.path)
}

func (b *BuilderRepo) initBuildersRepo() (*git.Repository, error) {
	repo, err := git.PlainClone(b.path, false, &git.CloneOptions{
		Depth:    1,
		URL:      originUrl,
		Progress: os.Stdout,
	})
	return repo, err
}

type builder struct {
	name string
	path string
}
