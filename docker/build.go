package docker

import (
	"archive/tar"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/superfly/flyctl/flyctl"
)

type BuildOptions struct {
	SourceDir string
	Tag       string
}

type BuildContext struct {
	reader    io.Reader
	writer    io.WriteCloser
	tar       *tar.Writer
	project   *flyctl.Project
	Tag       string
	SourceDir string
}

func (ctx *BuildContext) Read(p []byte) (n int, err error) {
	return ctx.reader.Read(p)
}

func NewBuildContext(sourceDir string, deploymentTag string) (*BuildContext, error) {
	reader, writer := io.Pipe()

	tw := tar.NewWriter(writer)

	ctx := &BuildContext{
		SourceDir: sourceDir,
		tar:       tw,
		reader:    reader,
		writer:    writer,
		Tag:       deploymentTag,
	}

	project, err := flyctl.LoadProject(sourceDir)
	if err != nil {
		return nil, err
	}

	ctx.project = project

	return ctx, nil
}

func (ctx *BuildContext) Close() error {
	return ctx.tar.Close()
}

func (ctx *BuildContext) Load() error {
	defer ctx.tar.Close()
	defer ctx.writer.Close()

	builderName := ctx.project.Builder()
	if builderName != "" {
		fmt.Println("Builder detected:", builderName)

		fmt.Println("Refreshing builders...")
		repo, err := NewBuilderRepo()
		if err != nil {
			return err
		}
		if err := repo.Sync(); err != nil {
			return err
		}

		builder, err := repo.GetBuilder(builderName)
		if err != nil {
			return err
		}
		if err := ctx.addFiles(builder.path); err != nil {
			return err
		}
	}
	if err := ctx.addFiles(ctx.SourceDir); err != nil {
		return err
	}
	return nil
}

func (ctx *BuildContext) BuildArgs() map[string]*string {
	var args = map[string]*string{}

	for k, v := range ctx.project.BuildArgs() {
		k = strings.ToUpper(k)
		// docker needs a string pointer. since ranges reuse variables we need to deref a copy
		val := v
		args[k] = &val
	}

	return args
}

func (ctx *BuildContext) addFiles(sourceDir string) error {
	err := filepath.Walk(sourceDir, func(fpath string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		switch {
		case info.IsDir() && info.Name() == ".git":
			return filepath.SkipDir
		}

		if info.IsDir() {
			return nil
		}

		relPath, err := filepath.Rel(sourceDir, fpath)
		if err != nil {
			return err
		}

		file, err := os.Open(fpath)
		if err != nil {
			return err
		}
		defer file.Close()

		hdr := &tar.Header{
			Name: relPath,
			Mode: 0600,
			Size: info.Size(),
		}

		if err := ctx.tar.WriteHeader(hdr); err != nil {
			return err
		}

		if _, err := io.Copy(ctx.tar, file); err != nil {
			return err
		}

		return nil
	})

	return err
}
