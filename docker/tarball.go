package docker

import (
	"io"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/docker/docker/pkg/archive"
)

type buildContext struct {
	workingDir string
}

func newBuildContext() (*buildContext, error) {
	tempDir, err := ioutil.TempDir("", "")
	if err != nil {
		return nil, err
	}
	return &buildContext{workingDir: tempDir}, nil
}

func (b *buildContext) Close() error {
	return os.RemoveAll(b.workingDir)
}

func (b *buildContext) AddSource(path string, excludes []string) error {
	reader, err := archive.TarWithOptions(path, &archive.TarOptions{
		Compression:     archive.Uncompressed,
		ExcludePatterns: excludes,
	})

	if err != nil {
		return err
	}

	return archive.Unpack(reader, b.workingDir, &archive.TarOptions{})
}

func (b *buildContext) AddFile(path string, r io.Reader) error {
	data, err := ioutil.ReadAll(r)
	if err != nil {
		return err
	}
	return ioutil.WriteFile(filepath.Join(b.workingDir, path), data, 0600)
}

func (b *buildContext) Archive() (*archive.TempArchive, error) {
	reader, err := archive.Tar(b.workingDir, archive.Gzip)
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	return archive.NewTempArchive(reader, "")
}
