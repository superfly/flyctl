package docker

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
)

func writeSourceContextTempFile(sources []string) (string, error) {
	file, err := ioutil.TempFile("", "*")
	if err != nil {
		return "", err
	}
	defer file.Close()
	gzWriter := gzip.NewWriter(file)

	if err := writeSourceContxt(gzWriter, sources); err != nil {
		return "", err
	}

	return file.Name(), nil
}

func writeSourceContxt(writer io.WriteCloser, sources []string) error {
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

			fmt.Println("Added", file.Name(), "=>", relPath)

			return nil
		})

		if err != nil {
			return err
		}
	}

	return nil
}
