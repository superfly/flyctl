package helpers

import (
	"os"
	"path"
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
	if path.Ext(pathname) != "" {
		pathname = filepath.Dir(pathname)
	}

	// TODO: this should probably be 0755
	return os.MkdirAll(pathname, 0o777)
}

// WriteFileAtomically writes data to a temporary file beside path, flushes it
// to disk, and renames it over path. A write that fails part way, for example
// on a full disk, leaves whatever was at path untouched. The rename also means
// a reader that already had the old file open never sees the new contents, so
// a file carrying a credential can safely be rewritten over a more permissive
// one. os.WriteFile truncates first and writes second, which both leaves an
// empty file behind on failure (an empty config reads back as a logged-out
// flyctl) and exposes the new contents through any open handle at the old
// mode.
func WriteFileAtomically(path string, data []byte, perm os.FileMode) (err error) {
	// Replace the file the link points to, not the link itself, as os.WriteFile
	// did. Users symlink config and credential files into place from a dotfiles
	// repository or a secrets mount.
	if resolved, e := filepath.EvalSymlinks(path); e == nil {
		path = resolved
	}

	f, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	tmp := f.Name()

	defer func() {
		if err != nil {
			_ = f.Close()
			_ = os.Remove(tmp)
		}
	}()

	if _, err = f.Write(data); err != nil {
		return err
	}
	if err = f.Sync(); err != nil {
		return err
	}
	if err = f.Chmod(perm); err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}

	return os.Rename(tmp, path)
}
