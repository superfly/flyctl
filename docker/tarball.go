package docker

import (
	"archive/tar"
	"compress/gzip"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/karrick/godirwalk"
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
		err := godirwalk.Walk(source, &godirwalk.Options{
			FollowSymbolicLinks: true,
			Unsorted:            true,
			Callback: func(osPathname string, de *godirwalk.Dirent) error {
				if de.IsDir() && de.Name() == ".git" {
					return filepath.SkipDir
				}

				relPath, err := filepath.Rel(source, osPathname)
				if err != nil {
					return err
				}

				if exclude(relPath, de.IsDir()) {
					if de.IsDir() {
						return filepath.SkipDir
					}
					return nil
				}

				if !de.ModeType().IsRegular() {
					return nil
				}

				file, err := os.Open(osPathname)
				if err != nil {
					return err
				}
				defer file.Close()

				info, err := file.Stat()
				if err != nil {
					return err
				}

				hdr, err := tar.FileInfoHeader(info, "")
				hdr.Name = relPath

				if err := tw.WriteHeader(hdr); err != nil {
					return err
				}

				if _, err := io.Copy(tw, file); err != nil {
					return err
				}

				return nil
			},
		})

		if err != nil {
			return err
		}
	}

	return nil
}
