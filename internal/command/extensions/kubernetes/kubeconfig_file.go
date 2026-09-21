package kubernetes

import (
	"fmt"
	"os"
)

// writeKubeconfig writes the cluster credential to path readable only by the
// current user. An existing file is truncated and narrowed to the same mode,
// since the mode passed to OpenFile only applies when the file is created.
func writeKubeconfig(path string, kubeconfig string) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()

	if err := f.Chmod(0o600); err != nil {
		return err
	}

	if _, err := f.Write([]byte(kubeconfig)); err != nil {
		return fmt.Errorf("failed to write kubeconfig to file %s, error: %w", path, err)
	}

	return nil
}
