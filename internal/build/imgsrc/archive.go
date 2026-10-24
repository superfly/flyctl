package imgsrc

import (
	"archive/tar"
	"bytes"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/docker/docker/pkg/fileutils"
	"github.com/moby/buildkit/frontend/dockerfile/dockerignore"
	"github.com/moby/go-archive"
	"github.com/moby/go-archive/compression"
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
		opts.Compression = compression.Gzip
	}

	sourcePath, err := fileutils.ReadSymlinkedDirectory(options.sourcePath)
	if err != nil {
		return nil, err
	}

	// tar can't represent irregular files, such as directory junctions on
	// Windows (php artisan storage:link creates one), so leave them out
	// instead of failing with "archive/tar: unknown file mode"
	irregular, err := irregularFiles(os.DirFS(sourcePath), options.exclusions)
	if err != nil {
		return nil, err
	}
	for _, name := range irregular {
		terminal.Warnf("Skipping %s: not a regular file, directory or symlink, so it can't be sent to the builder\n", name)
	}
	opts.ExcludePatterns = append(opts.ExcludePatterns, irregular...)

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

// irregularFiles returns the paths in fsys, outside of the excluded ones,
// whose file mode is irregular.
func irregularFiles(fsys fs.FS, excludes []string) ([]string, error) {
	pm, err := patternmatcher.New(excludes)
	if err != nil {
		return nil, err
	}

	var irregular []string
	err = fs.WalkDir(fsys, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || path == "." {
			return err
		}

		if excluded, _ := pm.MatchesOrParentMatches(path); excluded {
			// an exception pattern could bring back something below this directory
			if d.IsDir() && !pm.Exclusions() {
				return fs.SkipDir
			}

			return nil
		}

		if d.Type()&fs.ModeIrregular != 0 {
			irregular = append(irregular, path)
		}

		return nil
	})

	return irregular, err
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
