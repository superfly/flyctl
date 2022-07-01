package agent

import (
	"os"
	"path/filepath"
)

// TODO: deprecate
func PathToSocket() string {
	dir, err := os.UserHomeDir()
	if err != nil {
		panic(err)
	}

	return filepath.Join(dir, ".fly", "fly-agent.sock")
}

type Instances struct {
	Labels    []string
	Addresses []string
}
