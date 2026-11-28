//go:build !linux

package cli_test

import (
	"os"
	"testing"
)

func openPTY(t *testing.T) (master, slave *os.File) {
	t.Skip("pseudo-terminal helper is Linux only")

	return nil, nil
}
