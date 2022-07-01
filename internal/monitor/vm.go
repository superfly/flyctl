package monitor

import (
	"context"
	"time"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/retry"
)

func WaitForRunningVM(ctx context.Context, appName string, apiClient *api.Client) (err error) {
	for {
		var status *api.AppStatus

		fn := func() error {
			status, err = apiClient.GetAppStatus(ctx, appName, false)
			return err
		}

		if err := retry.Retry(fn, 3); err != nil {
			return err
		}

		if len(runningVMs(status.Allocations)) > 0 {
			return nil
		}

		if err := ctx.Err(); err != nil {
			return err
		}

		time.Sleep(1 * time.Second)
	}
}

func runningVMs(vms []*api.AllocationStatus) (out []*api.AllocationStatus) {
	for _, vm := range vms {
		if vm.DesiredStatus == "run" && !vm.Transitioning {
			out = append(out, vm)
		}
	}

	return
}
