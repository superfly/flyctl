package cmd

import (
	"context"
	"fmt"
	"sync"

	"github.com/superfly/flyctl/cmdctx"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/cmd/presenters"
	"github.com/superfly/flyctl/docstrings"
	"github.com/superfly/flyctl/internal/client"
	"github.com/superfly/flyctl/internal/deployment"
	"github.com/superfly/flyctl/internal/flyerr"
)

func newMonitorCommand(client *client.Client) *Command {
	ks := docstrings.Get("monitor")
	return BuildCommandKS(nil, runMonitor, ks, client, requireSession, requireAppName)
}

func runMonitor(commandContext *cmdctx.CmdContext) error {
	//var oldds *api.DeploymentStatus

	app, err := commandContext.Client.API().GetApp(commandContext.AppName)

	if err != nil {
		return fmt.Errorf("Failed to get app from context")
	}

	commandContext.Statusf("monitor", cmdctx.STITLE, "Monitoring Deployments for %s\n", app.Name)

	for {
		err := monitorDeployment(context.Background(), commandContext)
		if err != nil {
			return err
		}
	}

}

func monitorDeployment(ctx context.Context, commandContext *cmdctx.CmdContext) error {
	monitor := deployment.NewDeploymentMonitor(commandContext.Client.API(), commandContext.AppName)
	monitor.DeploymentStarted = func(idx int, d *api.DeploymentStatus) error {
		if idx > 0 {
			commandContext.StatusLn()
		}
		commandContext.Status("monitor", cmdctx.SINFO, presenters.FormatDeploymentSummary(d))
		return nil
	}
	monitor.DeploymentUpdated = func(d *api.DeploymentStatus, updatedAllocs []*api.AllocationStatus) error {
		commandContext.Status("monitor", cmdctx.SINFO, presenters.FormatDeploymentAllocSummary(d))

		if commandContext.GlobalConfig.GetBool("verbose") {
			for _, alloc := range updatedAllocs {
				commandContext.Status("monitor", cmdctx.SINFO, presenters.FormatAllocSummary(alloc))
			}
		}
		return nil
	}
	monitor.DeploymentFailed = func(d *api.DeploymentStatus, failedAllocs []*api.AllocationStatus) error {
		commandContext.Statusf("monitor", cmdctx.SINFO, "v%d %s - %s\n", d.Version, d.Status, d.Description)

		if len(failedAllocs) > 0 {
			commandContext.StatusLn()
			commandContext.Status("monitor", cmdctx.SERROR, "Failed Instances")

			x := make(chan *api.AllocationStatus)
			var wg sync.WaitGroup
			wg.Add(len(failedAllocs))

			for _, a := range failedAllocs {
				a := a
				go func() {
					defer wg.Done()
					alloc, err := commandContext.Client.API().GetAllocationStatus(commandContext.AppName, a.ID, 20)
					if err != nil {
						commandContext.Status("monitor", cmdctx.SERROR, "Error fetching instance", a.ID, err)
						return
					}
					x <- alloc
				}()
			}

			go func() {
				wg.Wait()
				close(x)
			}()

			count := 0
			for alloc := range x {
				count++
				commandContext.Statusf("monitor", cmdctx.SERROR, "\n  Failure #%d\n", count)
				err := commandContext.FrenderPrefix("    ",
					cmdctx.PresenterOption{
						Title: "Instance",
						Presentable: &presenters.Allocations{
							Allocations: []*api.AllocationStatus{alloc},
						},
						Vertical: true,
					},
					cmdctx.PresenterOption{
						Title: "Recent Events",
						Presentable: &presenters.AllocationEvents{
							Events: alloc.Events,
						},
					},
				)
				if err != nil {
					return err
				}

				commandContext.Status("monitor", cmdctx.STITLE, "Recent Logs")
				logPresenter := presenters.LogPresenter{HideAllocID: true, HideRegion: true, RemoveNewlines: true}
				logPresenter.FPrint(commandContext.Out, commandContext.OutputJSON(), alloc.RecentLogs)

			}

		}
		return nil
	}
	monitor.DeploymentSucceeded = func(d *api.DeploymentStatus) error {
		fmt.Fprintf(commandContext.Out, "v%d deployed successfully\n", d.Version)
		return nil
	}

	monitor.Start(ctx)

	if err := monitor.Error(); err != nil {
		fmt.Fprintf(commandContext.Out, "Monitor Error: %s", err)
	}

	if !monitor.Success() {
		return flyerr.ErrAbort
	}

	return nil
}
