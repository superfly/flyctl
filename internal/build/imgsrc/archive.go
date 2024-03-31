package imgsrc

import (
	"archive/tar"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/docker/docker/pkg/archive"
	"github.com/docker/docker/pkg/fileutils"
	"github.com/moby/buildkit/frontend/dockerfile/dockerignore"
	"github.com/moby/patternmatcher"
	"github.com/pkg/errors"
	"github.com/superfly/flyctl/terminal"
)

type archiveOptions struct {
	sourcePath string
	exclusions []string
	compressed bool
	additions  map[string][]byte
}

type ArchiveInfo struct {
	SizeInBytes int
	Content     []byte
}

func CreateArchive(dockerfile, workingDir, ignoreFile string, compressed bool) (*ArchiveInfo, error) {
	archiveOpts := archiveOptions{
		sourcePath: workingDir,
		compressed: compressed,
	}

	relativeDockerfilePath := ""

	// copy dockerfile into the archive if it's outside the context dir
	if !isPathInRoot(dockerfile, workingDir) {
		dockerfileData, err := os.ReadFile(dockerfile)
		if err != nil {
			return nil, errors.Wrap(err, "error reading Dockerfile")
		}
		archiveOpts.additions = map[string][]byte{
			"Dockerfile": dockerfileData,
		}
	} else {
		p, err := filepath.Rel(workingDir, dockerfile)
		if err != nil {
			return nil, err
		}
		relativeDockerfilePath = filepath.ToSlash(p)
	}

	excludes, err := readDockerignore(workingDir, ignoreFile, relativeDockerfilePath)
	if err != nil {
		return nil, errors.Wrap(err, "error reading .dockerignore")
	}
	archiveOpts.exclusions = excludes

	r, err := archiveDirectory(archiveOpts)
	if err != nil {
		return nil, err
	}
	contentBuf := new(bytes.Buffer)
	contentBuf.ReadFrom(r)
	content := contentBuf.Bytes()
	archiveInfo := &ArchiveInfo{
		SizeInBytes: len(content),
		Content:     content,
	}
	return archiveInfo, err
}

func archiveDirectory(options archiveOptions) (io.ReadCloser, error) {
	opts := &archive.TarOptions{
		ExcludePatterns: options.exclusions,
	}
	if options.compressed && len(options.additions) == 0 {
		opts.Compression = archive.Gzip
	}

	sourcePath, err := fileutils.ReadSymlinkedDirectory(options.sourcePath)
	if err != nil {
		return nil, err
	}
	r, err := archive.TarWithOptions(sourcePath, opts)
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

func readDockerignore(workingDir, ignoreFile, relativeDockerfilePath string) ([]string, error) {
	if ignoreFile == "" {
		ignoreFile = filepath.Join(workingDir, ".dockerignore")
	}

	file, err := os.Open(ignoreFile)
	if os.IsNotExist(err) {
		// ignore fly.toml by default if no dockerignore file is provided
		return []string{"fly.toml"}, nil
	} else if err != nil {
		return nil, err
	}
	defer func() {
		err := file.Close()
		if err != nil {
			terminal.Debugf("error closing dockerignore %s: %v\n", ignoreFile, err)
		}
	}()

	return parseDockerignore(file, relativeDockerfilePath)
}

func parseDockerignore(r io.Reader, dockerfile string) ([]string, error) {
	excludes, err := dockerignore.ReadAll(r)
	if err != nil {
		return nil, err
	}

	if match, _ := patternmatcher.Matches(".dockerignore", excludes); match {
		excludes = append(excludes, "!.dockerignore")
	}

	if dockerfile != "" {
		if match, _ := patternmatcher.Matches(dockerfile, excludes); match {
			excludes = append(excludes, "!"+dockerfile)
		}
	} else {
		if match, _ := patternmatcher.Matches("Dockerfile", excludes); match {
			excludes = append(excludes, "![Dd]ockerfile")
		} else if match, _ := patternmatcher.Matches("dockerfile", excludes); match {
			excludes = append(excludes, "![Dd]ockerfile")
		}
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
