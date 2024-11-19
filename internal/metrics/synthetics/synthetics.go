package synthetics

import (
	"context"

	"github.com/superfly/flyctl/internal/buildinfo"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/env"
	"github.com/superfly/flyctl/internal/logger"
	"github.com/superfly/flyctl/internal/task"
)

func StartSyntheticsMonitoringAgent(clientCtx context.Context) {
	log := logger.FromContext(clientCtx)

	if !shouldRunSyntheticsAgent(clientCtx) {
		log.Debug("synthetics agent disabled")
		return
	}

	task.FromContext(clientCtx).Run(func(taskCtx context.Context) {
		taskCtx, cancelTask := context.WithCancel(taskCtx)

		log.Debug("starting synthetics agent")
		go RunAgent(taskCtx)

		select {
		case <-taskCtx.Done():
		case <-clientCtx.Done():
		}

		log.Debug("synthetics agent stopped")
		cancelTask()
	})
}

func shouldRunSyntheticsAgent(ctx context.Context) bool {
	cfg := config.FromContext(ctx)

	// Do not run unless enabled
	if !cfg.SyntheticsAgent {
		return false
	}

	// don't run synthetics checks in a dev agent connecting to production flynthetics
	if buildinfo.IsDev() && cfg.SyntheticsBaseURLIsProduction() {
		return false
	}

	// Also do not run if it's CI, client will be gone by the time we try to run a probe
	if env.IsCI() {
		return false
	}

	return true
}
