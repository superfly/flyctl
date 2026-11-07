//go:build !windows

package helpers

// IsFileBusy reports whether err, from an operation on the file at path,
// means another process has the file open or is replacing it, so the same
// operation may succeed in a moment. Outside Windows an open file never
// blocks a rename, delete or read.
func IsFileBusy(string, error) bool { return false }
