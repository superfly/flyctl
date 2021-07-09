// +build windows

package cmd

import (
	"fmt"

	"github.com/superfly/flyctl/pkg/agent"
)

func StartAgent(cmd string) (*agent.Client, error) {
	return nil, fmt.Errorf("can't start agent on this platform (this is a bug, please report)")
}
