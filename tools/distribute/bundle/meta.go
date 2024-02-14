package bundle

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
	"github.com/superfly/flyctl/internal/release"
)

type buildArtifact struct {
	Name   string `json:"name"`
	Path   string `json:"path"`
	Goos   string `json:"goos"`
	Goarch string `json:"goarch"`
	Type   string `json:"type"`
	Extra  struct {
		Checksum string `json:"Checksum"`
		Format   string `json:"Format"`
	} `json:"extra"`
}

func loadJSONFile[T any](path string) (T, error) {
	var data T

	file, err := os.Open(path)
	if err != nil {
		return data, err
	}
	defer file.Close()

	err = json.NewDecoder(file).Decode(&data)
	if err != nil {
		return data, err
	}
	return data, nil
}

type Meta struct {
	Release *release.Meta `json:"release"`
	Assets  []Asset       `json:"assets"`
}

func (m Meta) Validate() error {
	if m.Release == nil {
		return errors.New("missing release metadata")
	}

	if m.Release.Version == nil {
		return errors.New("missing version number. make sure there's a verison in release.json")
	}

	if len(m.Assets) == 0 {
		return errors.New("no assets found")
	}

	return nil
}

type Asset struct {
	Name         string `json:"name"`
	Path         string `json:"path"`
	OS           string `json:"os"`
	Arch         string `json:"arch"`
	SHA256       string `json:"sha256"`
	ContentType  string `json:"content_type"`
	AbsolutePath string `json:"-"`
}

func releaseMeta(distDir string) (release.Meta, error) {
	return loadJSONFile[release.Meta](filepath.Join(distDir, "release.json"))
}

func assetsFromGoReleaserDist(distDir string) ([]Asset, error) {
	assets := []Asset{}

	filepath.WalkDir(distDir, func(path string, d os.DirEntry, err error) error {
		if filepath.Base(path) == "artifacts.json" {
			artifacts, err := loadJSONFile[[]buildArtifact]((path))
			if err != nil {
				return errors.Wrapf(err, "loading build artifacts from %q", path)
			}
			for _, artifact := range artifacts {
				if artifact.Type != "Archive" {
					continue
				}

				// goreleaser puts the dist/ prefix on the path. remove it
				relPath := strings.TrimPrefix(artifact.Path, "dist/")
				fullPath := filepath.Join(distDir, relPath)

				sha, err := sha256File(fullPath)
				if err != nil {
					return errors.Wrapf(err, "hashing %q", fullPath)
				}

				a := Asset{
					Name:         artifact.Name,
					Path:         artifact.Name,
					AbsolutePath: fullPath,
					OS:           artifact.Goos,
					Arch:         artifact.Goarch,
					SHA256:       sha,
				}

				if artifact.Extra.Format == "tar.gz" {
					a.ContentType = "application/gzip"
				} else if artifact.Extra.Format == "zip" {
					a.ContentType = "application/zip"
				} else {
					return errors.Errorf("unknown format %s", artifact.Extra.Format)
				}

				assets = append(assets, a)
			}
		}

		return nil
	})

	return assets, nil
}

func BuildMeta(distDir string) (Meta, error) {
	m := Meta{}

	releaseMeta, err := releaseMeta(distDir)
	if err != nil {
		return m, err
	}
	m.Release = &releaseMeta

	assets, err := assetsFromGoReleaserDist(distDir)
	if err != nil {
		return m, err
	}
	m.Assets = assets

	return m, nil
}

func sha256File(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", errors.Wrapf(err, "opening %q", path)
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", errors.Wrapf(err, "hashing %q", path)
	}

	return fmt.Sprintf("%x", hash.Sum(nil)), nil
}
