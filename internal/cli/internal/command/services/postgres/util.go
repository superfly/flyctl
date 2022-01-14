package postgres

import (
	"context"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/superfly/flyctl/internal/client"
	"github.com/superfly/flyctl/pkg/iostreams"
)

func waitForMachineState(ctx context.Context, client *client.Client, appId, machineId, state string) error {
	io := iostreams.FromContext(ctx)
	fmt.Fprintf(io.Out, "Waiting for machine %q to reach a healthy state.\n", machineId)

	timeout := time.After(2 * time.Minute)
	tick := time.Tick(1 * time.Second)
	for {
		select {
		case <-timeout:
			return fmt.Errorf("Timed out waiting for machine %s to reach state %s.\n", machineId, state)
		case <-tick:
			// TODO - Target specific machine rather than listing all.
			machines, err := client.API().ListMachines(ctx, appId, "")
			if err != nil {
				fmt.Println(err.Error())
			}
			if err == nil {
				for _, machine := range machines {
					if machine.ID == machineId {
						if machine.State == state {
							return nil
						}
						fmt.Fprintf(io.Out, "Machine state: %q, wanted: %q\n", machine.State, state)

					}
				}
			}
			time.Sleep(5 * time.Second)
		}
	}
}

// encodeCommand will base64 encode a command string so it can be passed
// in with  exec.Command.
func encodeCommand(command string) string {
	return base64.StdEncoding.Strict().EncodeToString([]byte(command))
}
