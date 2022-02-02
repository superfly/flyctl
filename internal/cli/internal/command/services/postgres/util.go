package postgres

import (
	"context"
	"encoding/base64"
	"fmt"
	"net"
	"time"

	"github.com/azazeal/pause"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/internal/logger"
	"github.com/superfly/flyctl/pkg/iostreams"
)

func waitForMachineState(parent context.Context, client *api.Client, appID, machineID, state string) error {
	io := iostreams.FromContext(parent)
	logger := logger.FromContext(parent)

	ctx, cancel := context.WithTimeout(parent, time.Duration(time.Minute*2))
	defer cancel()

	for ctx.Err() == nil {
		pause.For(ctx, time.Second)

		machines, err := client.ListMachines(ctx, appID, "")
		if err != nil {
			logger.Debugf("failed retrieving machines: %v", err)

			continue
		}

		for _, machine := range machines {
			if machine.ID == machineID {
				if machine.State == state {
					return nil
				}

				fmt.Fprintf(io.Out, "Machine state: %q, wanted: %q\n", machine.State, state)
			}
		}
	}

	return ctx.Err()
}

// encodeCommand will base64 encode a command string so it can be passed
// in with  exec.Command.
func encodeCommand(command string) string {
	return base64.StdEncoding.Strict().EncodeToString([]byte(command))
}

func machineIP(machine *api.Machine) string {
	ip := machine.IPs.Nodes[0].IP
	peerIP := net.ParseIP(ip)
	return peerIP.String()
}
