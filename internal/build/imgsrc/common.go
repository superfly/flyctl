package imgsrc

import (
	"fmt"

	"github.com/superfly/flyctl/helpers"
)

// ResolveDockerfileFromOptions resolves the dockerfile path from options or discovers it
func ResolveDockerfileFromOptions(opts ImageOptions) (string, error) {
	if opts.DockerfilePath != "" {
		if !helpers.FileExists(opts.DockerfilePath) {
			return "", fmt.Errorf("dockerfile '%s' not found", opts.DockerfilePath)
		}
		return opts.DockerfilePath, nil
	}

	return ResolveDockerfile(opts.WorkingDir), nil
}
