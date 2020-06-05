package helpers

import (
	"os"
	"path/filepath"
)

func FileExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir()
}

func DirectoryExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.IsDir()
}

func PathRelativeToCWD(path string) string {
	cwd, err := os.Getwd()
	if err != nil {
		return path
	}
	path, err = filepath.Rel(cwd, path)
	if err != nil {
		return path
	}
	return path
}

func MkdirAll(pathname string) error {
	pathname = filepath.Dir(pathname)

	return os.MkdirAll(pathname, 0777)
}
