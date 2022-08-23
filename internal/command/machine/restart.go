package machine

import (
	"context"
)

func RunRestart(ctx context.Context) (err error) {
	if err := runMachineStop(ctx); err != nil {
		return err
	}

	return runMachineStart(ctx)
}
