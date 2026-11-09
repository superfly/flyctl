package kubernetes

import (
	"fmt"

	"github.com/superfly/flyctl/helpers"
)

// writeKubeconfig writes the cluster credential to path readable only by the
// current user, replacing any existing file rather than rewriting it in place.
func writeKubeconfig(path string, kubeconfig string) error {
	if err := helpers.WriteFileAtomically(path, []byte(kubeconfig), 0o600); err != nil {
		return fmt.Errorf("failed to write kubeconfig to file %s, error: %w", path, err)
	}
	return nil
}
