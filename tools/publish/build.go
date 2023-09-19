package main

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/hashicorp/go-multierror"
	"github.com/pkg/errors"
	"github.com/superfly/flyctl/internal/version"
)

type ReleaseMeta struct {
	Version   string    `json:"version"`
	Track     string    `json:"track"`
	GitCommit string    `json:"git_commit"`
	GitTag    string    `json:"git_tag"`
	GitBranch string    `json:"git_branch"`
	BuildTime time.Time `json:"build_time"`
	Assets    []Asset   `json:"assets"`
}

type Asset struct {
	Name        string `json:"name"`
	Path        string `json:"path"`
	OS          string `json:"os"`
	Arch        string `json:"arch"`
	SHA256      string `json:"sha256"`
	ContentType string `json:"content_type"`
}

func createReleaseArchive(srcDir string, w io.WriteCloser) (*ReleaseMeta, error) {
	archive := newArchive(w)
	defer archive.Close()

	buildInfo, err := loadJSONFile[buildInfo](filepath.Join(srcDir, "metadata.json"))
	if err != nil {
		return nil, errors.Wrap(err, "loading build info")
	}

	version, err := version.Parse(buildInfo.Version)
	if err != nil {
		return nil, err
	}

	buildTime, err := time.Parse(time.RFC3339, buildInfo.Date)
	if err != nil {
		return nil, err
	}

	releaseMeta := &ReleaseMeta{
		Version:   version.String(),
		Track:     version.Track,
		GitCommit: buildInfo.Commit,
		GitTag:    buildInfo.Tag,
		GitBranch: "wat",
		BuildTime: buildTime,
	}

	artifacts, err := loadJSONFile[[]buildArtifact]((filepath.Join(srcDir, "artifacts.json")))
	if err != nil {
		return nil, errors.Wrap(err, "loading build artifacts")
	}

	fmt.Println("Distributing Build")
	fmt.Println("  Version:", buildInfo.Version)
	fmt.Println("  Tag:", buildInfo.Tag)
	fmt.Println("  Previous Tag:", buildInfo.PreviousTag)
	fmt.Println("  Commit:", buildInfo.Commit)
	fmt.Println("  Date:", buildInfo.Date)

	fmt.Println("  Artifacts:")

	for _, artifact := range artifacts {
		if artifact.Type == "Archive" {
			fmt.Println("    ", artifact.Name)

			// goreleaser puts the dist/ prefix on the path. remove it
			relPath := strings.TrimPrefix(artifact.Path, "dist/")
			fullPath := filepath.Join(srcDir, relPath)

			a := Asset{
				Name:   artifact.Name,
				Path:   relPath,
				OS:     artifact.Goos,
				Arch:   artifact.Goarch,
				SHA256: strings.TrimPrefix(artifact.Extra.Checksum, "sha256:"),
			}

			if artifact.Extra.Format == "tar.gz" {
				a.ContentType = "application/gzip"
			} else if artifact.Extra.Format == "zip" {
				a.ContentType = "application/zip"
			} else {
				return nil, errors.Errorf("unknown format %s", artifact.Extra.Format)
			}

			releaseMeta.Assets = append(releaseMeta.Assets, a)

			if err := archive.WriteFile(fullPath); err != nil {
				return nil, errors.Wrap(err, "writing artifact")
			}
		}
	}

	if err := archive.WriteJSON(releaseMeta, "meta.json"); err != nil {
		return nil, errors.Wrap(err, "writing meta.json")
	}

	return releaseMeta, nil
}

func newArchive(w io.WriteCloser) *archive {
	gw := gzip.NewWriter(w)
	tw := tar.NewWriter(gw)
	return &archive{tw, gw}
}

type archive struct {
	tw *tar.Writer
	gq *gzip.Writer
}

func (a *archive) WriteJSON(thing any, name string) error {
	data, err := json.Marshal(thing)
	if err != nil {
		return err
	}

	hdr := &tar.Header{
		Name: name,
		Mode: 0644,
		Size: int64(len(data)),
	}
	if err := a.tw.WriteHeader(hdr); err != nil {
		return err
	}

	if _, err := a.tw.Write(data); err != nil {
		return err
	}

	return nil
}

func (a *archive) WriteFile(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return err
	}

	header, err := tar.FileInfoHeader(info, info.Name())
	if err != nil {
		return err
	}

	err = a.tw.WriteHeader(header)
	if err != nil {
		return err
	}

	_, err = io.Copy(a.tw, file)
	if err != nil {
		return err
	}

	return nil
}

func (a *archive) Close() error {
	return multierror.Append(nil, a.tw.Close(), a.gq.Close())
}
