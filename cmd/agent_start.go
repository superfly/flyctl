// +build !windows

package cmd

import (
	"fmt"
	"os/exec"
	"syscall"
	"time"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/pkg/agent"
)

func StartAgent(api *api.Client, command string) (*agent.Client, error) {
	cmd := exec.Command(command, "agent", "daemon-start")
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
		Pgid:    0,
	}

	if err := cmd.Start(); err != nil {
		return nil, err
	}

	// this is gross placeholder logic

	for i := 0; i < 5; i++ {
		time.Sleep(100 * time.Millisecond)

		c, err := agent.DefaultClient(api)
		if err == nil {
			_, err := c.Ping()
			if err == nil {
				return c, nil
			}
		}
	}

	return nil, fmt.Errorf("couldn't establish connection to Fly Agent")
}
