// +build !windows

package agent

import (
	"fmt"
	"os/exec"
	"syscall"
	"time"

	"github.com/superfly/flyctl/api"
)

func StartDaemon(api *api.Client, command string) (*Client, error) {
	cmd := exec.Command(command, "agent", "daemon-start")
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
		Pgid:    0,
	}

	if err := cmd.Start(); err != nil {
		return nil, err
	}

	// this is gross placeholder logic

	for i := 0; i < 20; i++ {
		time.Sleep(100 * time.Millisecond)

		c, err := DefaultClient(api)
		if err == nil {
			_, err := c.Ping()
			if err == nil {
				return c, nil
			}
		}
	}

	return nil, fmt.Errorf("couldn't establish connection to Fly Agent")
}
