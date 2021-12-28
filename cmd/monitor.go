package cmd

import (
	"fmt"
	"sync"

	"github.com/superfly/flyctl/cmdctx"
	"github.com/superfly/flyctl/pkg/logs"
	"github.com/superfly/flyctl/terminal"

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

func runMonitor(commandContext *cmdctx.CmdContext) (error error) {
	ctx := commandContext.Command.Context()

	app, err := commandContext.Client.API().GetApp(ctx, commandContext.AppName)

	if err != nil {
		return fmt.Errorf("Failed to get app from context")
	}

	commandContext.Statusf("monitor", cmdctx.STITLE, "Monitoring Deployments for %s\n", app.Name)

	for {
		err := monitorDeployment(*commandContext)
		if err != nil {
			return
		}
	}

}

func monitorDeployment(cmdCtx cmdctx.CmdContext) error {
	monitor := deployment.NewDeploymentMonitor(cmdCtx.Client.API(), cmdCtx.AppName)
	monitor.DeploymentStarted = func(idx int, d *api.DeploymentStatus) error {
		if idx > 0 {
			fmt.Println()
		}
		cmdCtx.Status("monitor", cmdctx.SINFO, presenters.FormatDeploymentSummary(d))
		return nil
	}
	monitor.DeploymentUpdated = func(d *api.DeploymentStatus, updatedAllocs []*api.AllocationStatus) error {
		cmdCtx.Status("monitor", cmdctx.SINFO, presenters.FormatDeploymentAllocSummary(d))

		if cmdCtx.GlobalConfig.GetBool("verbose") {
			for _, alloc := range updatedAllocs {
				cmdCtx.Status("monitor", cmdctx.SINFO, presenters.FormatAllocSummary(alloc))
			}
		}
		return nil
	}
	monitor.DeploymentFailed = func(d *api.DeploymentStatus, failedAllocs []*api.AllocationStatus) error {
		cmdCtx.Statusf("monitor", cmdctx.SINFO, "v%d %s - %s\n", d.Version, d.Status, d.Description)

		if len(failedAllocs) > 0 {
			cmdCtx.StatusLn()
			cmdCtx.Status("monitor", cmdctx.SERROR, "Failed Instances")

			x := make(chan *api.AllocationStatus)
			var wg sync.WaitGroup
			wg.Add(len(failedAllocs))

			for _, a := range failedAllocs {
				a := a
				go func() {
					defer wg.Done()
					alloc, err := cmdCtx.Client.API().GetAllocationStatus(cmdCtx.Command.Context(), cmdCtx.AppName, a.ID, 20)
					if err != nil {
						cmdCtx.Status("monitor", cmdctx.SERROR, "Error fetching instance", a.ID, err)
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
				cmdCtx.Statusf("monitor", cmdctx.SERROR, "\n  Failure #%d\n", count)
				err := cmdCtx.FrenderPrefix("    ",
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

				cmdCtx.Status("monitor", cmdctx.STITLE, "Recent Logs")
				logPresenter := presenters.LogPresenter{HideAllocID: true, HideRegion: true, RemoveNewlines: true}
				terminal.Debug("logs", "Fetching logs for %s", alloc.ID)
				for _, e := range alloc.RecentLogs {
					entry := logs.LogEntry{
						Instance:  e.Instance,
						Level:     e.Level,
						Message:   e.Message,
						Region:    e.Region,
						Timestamp: e.Timestamp,
						Meta:      e.Meta,
					}
					logPresenter.FPrint(cmdCtx.Out, cmdCtx.OutputJSON(), entry)
				}

			}

		}
		return nil
	}
	monitor.DeploymentSucceeded = func(d *api.DeploymentStatus) error {
		fmt.Fprintf(cmdCtx.Out, "v%d deployed successfully\n", d.Version)
		return nil
	}

	monitor.Start(cmdCtx.Command.Context())

	if err := monitor.Error(); err != nil {
		fmt.Fprintf(cmdCtx.Out, "Monitor Error: %s", err)
	}

	if !monitor.Success() {
		return flyerr.ErrAbort
	}

	return nil
}
