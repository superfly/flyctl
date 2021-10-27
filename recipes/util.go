package recipes

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/superfly/flyctl/internal/client"
)

func WaitForMachineState(ctx context.Context, client *client.Client, appId, machineId, state string) error {
	fmt.Printf("Waiting for machine %q to reach a healthy state.\n", machineId)
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
						fmt.Printf("Machine state: %q, wanted: %q\n", machine.State, state)

					}
				}
			}
			time.Sleep(5 * time.Second)
		}
	}
}

func GenerateSecureToken(length int) string {
	b := make([]byte, length)
	if _, err := rand.Read(b); err != nil {
		return ""
	}
	return hex.EncodeToString(b)
}

// encodeCommand will base64 encode a command string so it can be passed
// in with  exec.Command.
func EncodeCommand(command string) string {
	return base64.StdEncoding.Strict().EncodeToString([]byte(command))
}
