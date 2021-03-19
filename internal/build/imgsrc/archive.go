package imgsrc

import (
	"archive/tar"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/docker/docker/builder/dockerignore"
	"github.com/docker/docker/pkg/archive"
	"github.com/docker/docker/pkg/fileutils"
)

type archiveOptions struct {
	sourcePath string
	exclusions []string
	compressed bool
	additions  map[string][]byte
}

func archiveDirectory(options archiveOptions) (io.ReadCloser, error) {
	opts := &archive.TarOptions{
		ExcludePatterns: options.exclusions,
	}
	if options.compressed {
		opts.Compression = archive.Gzip
	}

	r, err := archive.TarWithOptions(options.sourcePath, opts)
	if err != nil {
		return nil, err
	}

	if options.additions != nil {
		mods := map[string]archive.TarModifierFunc{}
		for name, contents := range options.additions {
			mods[name] = func(path string, header *tar.Header, content io.Reader) (*tar.Header, []byte, error) {
				newHeader := &tar.Header{
					Name: name,
					Size: int64(len(contents)),
				}

				return newHeader, contents, nil
			}
		}

		r = archive.ReplaceFileTarWrapper(r, mods)
	}

	return r, nil
}

func readDockerignore(workingDir string) ([]string, error) {
	file, err := os.Open(filepath.Join(workingDir, ".dockerignore"))
	if os.IsNotExist(err) {
		return []string{}, nil
	} else if err != nil {
		return nil, err
	}
	defer file.Close()

	return parseDockerignore(file)
}

func parseDockerignore(r io.Reader) ([]string, error) {
	excludes, err := dockerignore.ReadAll(r)
	if err != nil {
		return nil, err
	}

	if match, _ := fileutils.Matches("fly.toml", excludes); !match {
		excludes = append(excludes, "fly.toml")
	}

	if match, _ := fileutils.Matches(".dockerignore", excludes); match {
		excludes = append(excludes, "!.dockerignore")
	}

	if match, _ := fileutils.Matches("Dockerfile", excludes); match {
		excludes = append(excludes, "![Dd]ockerfile")
	}

	if match, _ := fileutils.Matches("dockerfile", excludes); match {
		excludes = append(excludes, "![Dd]ockerfile")
	}

	return excludes, nil
}

func isPathInRoot(target, rootDir string) bool {
	rootDir, _ = filepath.Abs(rootDir)
	if !filepath.IsAbs(target) {
		target = filepath.Join(rootDir, target)
	}

	rel, err := filepath.Rel(rootDir, target)
	if err != nil {
		return false
	}
	return !strings.HasPrefix(filepath.ToSlash(rel), "../")
}
