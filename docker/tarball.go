package docker

import (
	"archive/tar"
	"compress/gzip"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
)

type excludeMatcher func(path string, isDir bool) bool

func noopMatcher(path string, isDir bool) bool {
	return false
}

func writeSourceContextTempFile(sources []string, exclude excludeMatcher) (string, error) {
	file, err := ioutil.TempFile("", "*")
	if err != nil {
		return "", err
	}
	defer file.Close()
	gzWriter := gzip.NewWriter(file)

	if err := writeSourceContxt(gzWriter, sources, exclude); err != nil {
		return "", err
	}

	return file.Name(), nil
}

func writeSourceContxt(writer io.WriteCloser, sources []string, exclude excludeMatcher) error {
	tw := tar.NewWriter(writer)
	defer writer.Close()
	defer tw.Close()

	for _, source := range sources {
		err := filepath.Walk(source, func(fpath string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}

			switch {
			case info.IsDir() && info.Name() == ".git":
				return filepath.SkipDir
			}

			if exclude(fpath, info.IsDir()) {
				if info.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}

			if info.IsDir() {
				return nil
			}

			relPath, err := filepath.Rel(source, fpath)
			if err != nil {
				return err
			}

			file, err := os.Open(fpath)
			if err != nil {
				return err
			}
			defer file.Close()

			info, _ = file.Stat()

			hdr, err := tar.FileInfoHeader(info, "")
			hdr.Name = relPath

			if err := tw.WriteHeader(hdr); err != nil {
				return err
			}

			if _, err := io.Copy(tw, file); err != nil {
				return err
			}

			return nil
		})

		if err != nil {
			return err
		}
	}

	return nil
}
