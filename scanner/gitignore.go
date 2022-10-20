package scanner

import (
	"io/fs"
	"path/filepath"
)

func FindGitignores(root string) []string {
	gitignores := make([]string, 0)
	filepath.Walk(root, func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if m, err := filepath.Match(".gitignore", filepath.Base(path)); err != nil {
			return err
		} else if m {
			gitignores = append(gitignores, path)
		}
		return nil
	})
	return gitignores
}
