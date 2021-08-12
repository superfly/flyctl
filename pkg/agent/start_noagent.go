// +build windows

package agent

import (
	"fmt"

	"github.com/superfly/flyctl/api"
)

func StartDaemon(api *api.Client, cmd string) (*Client, error) {
	return nil, fmt.Errorf("can't start agent on this platform (this is a bug, please report)")
}
