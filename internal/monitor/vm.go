package monitor

import (
	"context"
	"time"

	"github.com/superfly/flyctl/api"
)

type UpdateFn func(status string)

func WaitForRunningVM(ctx context.Context, appName string, apiClient *api.Client, timeout time.Duration, update UpdateFn) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	done := make(chan error)
	errorCount := 0

	go func() {
		for {
			status, err := apiClient.GetAppStatus(appName, false)
			if err != nil {
				errorCount += 1
				if errorCount < 3 {
					continue
				}
				done <- err
			}

			isRunning := false

			var currentStatus string

			if len(runningVMs(status.Allocations)) > 0 {
				currentStatus = "running"
				isRunning = true
			} else {
				currentStatus = "starting"
			}

			update(currentStatus)

			if isRunning {
				done <- nil
				break
			}

			time.Sleep(1 * time.Second)
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return false, context.Canceled
		case err := <-done:
			return err == nil, err
		}
	}
}

func runningVMs(vms []*api.AllocationStatus) (out []*api.AllocationStatus) {
	for _, vm := range vms {
		if vm.LatestVersion && vm.DesiredStatus == "run" && !vm.Transitioning {
			out = append(out, vm)
		}
	}

	return
}
