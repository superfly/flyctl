package docker

import (
	"io/ioutil"
	"os"

	"github.com/docker/docker/pkg/archive"
)

type BuildContext struct {
	workingDir string
}

func NewBuildContext() (*BuildContext, error) {
	tempDir, err := ioutil.TempDir("", "")
	if err != nil {
		return nil, err
	}
	return &BuildContext{workingDir: tempDir}, nil
}

func (b *BuildContext) Close() error {
	return os.RemoveAll(b.workingDir)
}

func (b *BuildContext) AddSource(path string, excludes []string) error {
	reader, err := archive.TarWithOptions(path, &archive.TarOptions{
		Compression:     archive.Uncompressed,
		ExcludePatterns: excludes,
	})

	if err != nil {
		return err
	}

	return archive.Unpack(reader, b.workingDir, &archive.TarOptions{})
}

func (b *BuildContext) Archive() (*archive.TempArchive, error) {
	reader, err := archive.Tar(b.workingDir, archive.Gzip)
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	return archive.NewTempArchive(reader, "")
}
