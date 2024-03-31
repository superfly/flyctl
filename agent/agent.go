package agent

import (
	"path/filepath"

	"github.com/superfly/flyctl/helpers"
)

// TODO: deprecate
func PathToSocket() string {
	dir, err := helpers.GetConfigDirectory()
	if err != nil {
		panic(err)
	}

	return filepath.Join(dir, "fly-agent.sock")
}

type Instances struct {
	Labels    []string
	Addresses []string
}
