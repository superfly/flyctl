// +build !windows

package agent

import (
	"bufio"
	"bytes"
	"context"
	"os/exec"
	"regexp"
	"syscall"
	"time"

	"github.com/pkg/errors"
	"github.com/superfly/flyctl/api"
)

func StartDaemon(ctx context.Context, api *api.Client, command string) (*Client, error) {
	cmd := exec.Command(command, "agent", "daemon-start")
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
		Pgid:    0,
	}

	// buffer stdout and stderr from the daemon process. If it
	// includes "OK <pid>" we know it started successfully.
	// Otherwise we know it failed and we can include the output with the
	// returnred error so it can be displayed to the user
	out, err := cmd.StdoutPipe()
	if err != nil {
		panic(err)
	}

	startCh := make(chan error, 1)

	go func() {
		var output bytes.Buffer
		r := regexp.MustCompile(`\AOK \d+\z`)

		scanner := bufio.NewScanner(out)
		scanner.Split(bufio.ScanLines)

		var ok bool
		for scanner.Scan() {
			if r.Match(scanner.Bytes()) {
				ok = true
				break
			}
			if output.Len() > 0 {
				output.WriteByte(byte('\n'))
			}
			output.Write(scanner.Bytes())
		}

		if ok {
			startCh <- nil
			return
		}

		startCh <- &AgentStartError{Output: output.String()}
	}()

	// run the command while the goroutine captures output
	if err := cmd.Start(); err != nil {
		return nil, err
	}

	// wait for the output to include "OK <pid>" or EOF
	if startErr := <-startCh; startErr != nil {
		return nil, startErr
	}

	client, err := waitForClient(ctx, api)
	if err != nil {
		return nil, errors.Wrap(err, "couldn't establish connection to Fly Agent")
	}

	return client, nil
}

func waitForClient(ctx context.Context, api *api.Client) (*Client, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Second*5)
	defer cancel()

	respCh := make(chan *Client, 1)

	go func() {
		for {
			time.Sleep(100 * time.Millisecond)

			c, err := DefaultClient(api)
			if err == nil {
				_, err := c.Ping(ctx)
				if err == nil {
					respCh <- c
					break
				}
			}
		}
	}()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case client := <-respCh:
		return client, nil
	}
}
