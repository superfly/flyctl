package imgsrc

import (
	"archive/tar"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/docker/docker/pkg/archive"
	"github.com/stretchr/testify/assert"
)

func newTestDir(filenames ...string) (tempDir string, err error) {
	tempDir, err = os.MkdirTemp("", "")
	if err != nil {
		return
	}

	defer func() {
		if err != nil {
			os.RemoveAll(tempDir)
		}
	}()

	for _, filename := range filenames {
		content := []byte(filename)
		filename = filepath.Join(tempDir, filename)
		err = os.MkdirAll(filepath.Dir(filename), 0o777)
		if err != nil {
			return
		}

		err = os.WriteFile(filename, content, 0o777)
		if err != nil {
			return
		}

	}

	return
}

func unpackTar(r io.ReadCloser) ([]string, map[string][]byte, error) {
	tr := tar.NewReader(r)

	names := []string{}
	out := map[string][]byte{}

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, nil, err
		}

		if hdr.Typeflag != tar.TypeReg {
			continue
		}

		names = append(names, hdr.Name)

		data, err := io.ReadAll(tr)
		if err != nil {
			return nil, nil, err
		}
		out[hdr.Name] = data
	}

	return names, out, nil
}

func TestArchiver(t *testing.T) {
	testDir, err := newTestDir("a.jpg", "content/foo.md", "images/a.jpg", "images/b.jpg")
	assert.NoError(t, err)
	defer os.RemoveAll(testDir)

	r, err := archiveDirectory(archiveOptions{
		sourcePath: testDir,
	})
	assert.NoError(t, err)

	names, _, err := unpackTar(r)
	assert.NoError(t, err)

	assert.ElementsMatch(t, names, []string{"a.jpg", "content/foo.md", "images/a.jpg", "images/b.jpg"})
}

func TestArchiverExcludes(t *testing.T) {
	testDir, err := newTestDir("a.jpg", "content/foo.md", "images/a.jpg", "images/b.jpg")
	assert.NoError(t, err)
	defer os.RemoveAll(testDir)

	r, err := archiveDirectory(archiveOptions{
		sourcePath: testDir,
		exclusions: []string{"**/*.jpg"},
	})
	assert.NoError(t, err)

	names, _, err := unpackTar(r)
	assert.NoError(t, err)

	assert.ElementsMatch(t, names, []string{"content/foo.md"})
}

func TestArchiverAdditions(t *testing.T) {
	testDir, err := newTestDir("a.jpg", "content/foo.md", "images/a.jpg", "images/b.jpg")
	assert.NoError(t, err)
	defer os.RemoveAll(testDir)

	r, err := archiveDirectory(archiveOptions{
		sourcePath: testDir,
		additions: map[string][]byte{
			"Dockerfile": []byte("this is a dockerfile"),
		},
	})
	assert.NoError(t, err)

	names, contents, err := unpackTar(r)
	assert.NoError(t, err)

	assert.Contains(t, names, "Dockerfile")
	assert.Equal(t, []byte("this is a dockerfile"), contents["Dockerfile"])
}

func TestArchiverCompression(t *testing.T) {
	testDir, err := newTestDir("a.jpg", "content/foo.md", "images/a.jpg", "images/b.jpg")
	assert.NoError(t, err)
	defer os.RemoveAll(testDir)

	r, err := archiveDirectory(archiveOptions{sourcePath: testDir, compressed: true})
	assert.NoError(t, err)
	data, err := io.ReadAll(r)
	assert.NoError(t, err)
	assert.Equal(t, archive.Gzip, archive.DetectCompression(data))

	r, err = archiveDirectory(archiveOptions{sourcePath: testDir, compressed: false})
	assert.NoError(t, err)
	data, err = io.ReadAll(r)
	assert.NoError(t, err)
	assert.Equal(t, archive.Uncompressed, archive.DetectCompression(data))
}

func TestArchiverNoCompressionWithAdditions(t *testing.T) {
	testDir, err := newTestDir("a.jpg", "content/foo.md", "images/a.jpg", "images/b.jpg")
	assert.NoError(t, err)
	defer os.RemoveAll(testDir)

	r, err := archiveDirectory(archiveOptions{sourcePath: testDir, compressed: true, additions: map[string][]byte{
		"Dockerfile": []byte("this is a dockerfile"),
	}})
	assert.NoError(t, err)
	data, err := io.ReadAll(r)
	assert.NoError(t, err)
	assert.Equal(t, archive.Uncompressed, archive.DetectCompression(data))
}

func TestParseDockerignore(t *testing.T) {
	cases := map[string][]string{
		"node_modules\n*.jpg":                {"node_modules", "*.jpg"},
		"node_modules\n*.jpg\nDockerfile":    {"node_modules", "*.jpg", "Dockerfile", "![Dd]ockerfile"},
		"node_modules\n*.jpg\ndockerfile":    {"node_modules", "*.jpg", "dockerfile", "![Dd]ockerfile"},
		"node_modules\n*.jpg\n.dockerignore": {"node_modules", "*.jpg", ".dockerignore", "!.dockerignore"},
	}

	for input, expected := range cases {
		excludes, err := parseDockerignore(strings.NewReader(input))
		assert.NoError(t, err)
		assert.Equal(t, expected, excludes, input)
	}
}

func TestIsPathInRoot(t *testing.T) {
	cases := []struct {
		filename string
		rootDir  string
		rooted   bool
	}{
		{filename: "Dockerfile", rootDir: "/a/b/c", rooted: true},
		{filename: "../Dockerfile", rootDir: "/a/b/c", rooted: false},
		{filename: "path/to/Dockerfile", rootDir: "/a/b/c", rooted: true},
		{filename: "/a/b/c/Dockerfile", rootDir: "/a/b/c", rooted: true},
		{filename: "/a/b/c/path/to/Dockerfile", rootDir: "/a/b/c", rooted: true},
		{filename: "/Dockerfile", rootDir: "/a/b/c", rooted: false},
		{filename: "/a/b/c/../Dockerfile", rootDir: "/a/b/c", rooted: false},
		{filename: "/a/b/c/path/to/../../../Dockerfile", rootDir: "/a/b/c", rooted: false},
	}

	for _, c := range cases {
		assert.Equal(t, c.rooted, isPathInRoot(c.filename, c.rootDir), "target: %s root:%s", c.filename, c.rootDir)
	}
}
