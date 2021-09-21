package monitor

import (
	"context"
	"time"

	"github.com/pkg/errors"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/pkg/retry"
)

func WaitForRunningMachine(ctx context.Context, appName string, machineID string, apiClient *api.Client) (*api.Machine, error) {
	var err error
	for {
		var machines []*api.Machine

		fn := func() error {
			machines, err = apiClient.ListMachines(appName, "")
			return err
		}

		if err := retry.Retry(fn, 3); err != nil {
			return nil, err
		}

		for _, machine := range machines {
			if machine.ID == machineID {
				if machine.State == "started" {
					return machine, nil
				}
				if machine.State != "starting" {
					return nil, errors.Errorf("machine isn't starting, state: %s", machine.State)
				}
			}
		}

		if err := ctx.Err(); err != nil {
			return nil, err
		}

		time.Sleep(1 * time.Second)
	}
}
