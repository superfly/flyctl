package bundle

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"io"
	"os"

	"github.com/hashicorp/go-multierror"
	"github.com/pkg/errors"
)

func CreateReleaseBundle(meta Meta, w io.WriteCloser) (*archive, error) {
	archive := newArchive(w)
	defer archive.Close()

	for _, asset := range meta.Assets {
		if err := archive.WriteFile(asset.AbsolutePath); err != nil {
			return nil, errors.Wrapf(err, "bundling asset %q", asset.Name)
		}
	}

	if err := archive.WriteJSON(meta, "meta.json"); err != nil {
		return nil, errors.Wrap(err, "writing meta.json")
	}

	return archive, nil
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
