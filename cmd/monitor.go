package cmd

import (
	"context"
	"fmt"
	"os"
	"sync"

	"github.com/logrusorgru/aurora"
	"github.com/segmentio/textio"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/cmd/presenters"
	"github.com/superfly/flyctl/docstrings"
	"github.com/superfly/flyctl/internal/deployment"
)

func newMonitorCommand() *Command {
	ks := docstrings.Get("monitor")
	return BuildCommand(nil, runMonitor, ks.Usage, ks.Short, ks.Long, os.Stdout, requireSession, requireAppName)
}

func runMonitor(ctx *CmdContext) error {
	//var oldds *api.DeploymentStatus

	app, err := ctx.Client.API().GetApp(ctx.AppName)

	if err != nil {
		return fmt.Errorf("Failed to get app from context")
	}

	fmt.Printf("Monitoring Deployments for %s\n", app.Name)

	for {
		monitorDeployment(context.Background(), ctx)
	}

	return nil
}

func monitorDeployment(ctx context.Context, cc *CmdContext) error {

	//interactive := isatty.IsTerminal(os.Stdout.Fd())

	monitor := deployment.NewDeploymentMonitor(cc.Client.API(), cc.AppName)
	monitor.DeploymentStarted = func(idx int, d *api.DeploymentStatus) error {
		if idx > 0 {
			fmt.Fprintln(cc.Out)
		}
		fmt.Fprintln(cc.Out, presenters.FormatDeploymentSummary(d))
		return nil
	}
	monitor.DeploymentUpdated = func(d *api.DeploymentStatus, updatedAllocs []*api.AllocationStatus) error {
		fmt.Fprintln(cc.Out, presenters.FormatDeploymemntAllocSummary(d))

		if cc.Verbose {
			for _, alloc := range updatedAllocs {
				fmt.Fprintln(cc.Out, presenters.FormatAllocSummary(alloc))
			}
		}
		return nil
	}
	monitor.DeploymentFailed = func(d *api.DeploymentStatus, failedAllocs []*api.AllocationStatus) error {
		fmt.Fprintf(cc.Out, "v%d %s - %s\n", d.Version, d.Status, d.Description)

		if len(failedAllocs) > 0 {
			fmt.Fprintln(cc.Out)
			fmt.Fprintln(cc.Out, "Failed Allocations")

			x := make(chan *api.AllocationStatus)
			var wg sync.WaitGroup
			wg.Add(len(failedAllocs))

			for _, a := range failedAllocs {
				a := a
				go func() {
					defer wg.Done()
					alloc, err := cc.Client.API().GetAllocationStatus(cc.AppName, a.ID, 20)
					if err != nil {
						fmt.Println("Error fetching alloc", a.ID, err)
						return
					}
					x <- alloc
				}()
			}

			go func() {
				wg.Wait()
				close(x)
			}()

			p := textio.NewPrefixWriter(cc.Out, "    ")

			count := 0
			for alloc := range x {
				count++
				fmt.Fprintf(cc.Out, "\n  Failure #%d\n", count)
				err := cc.RenderViewW(p,
					PresenterOption{
						Title: "Allocation",
						Presentable: &presenters.Allocations{
							Allocations: []*api.AllocationStatus{alloc},
						},
						Vertical: true,
					},
					PresenterOption{
						Title: "Recent Events",
						Presentable: &presenters.AllocationEvents{
							Events: alloc.Events,
						},
					},
				)
				if err != nil {
					return err
				}

				fmt.Fprintln(p, aurora.Bold("Recent Logs"))
				logPresenter := presenters.LogPresenter{HideAllocID: true, HideRegion: true, RemoveNewlines: true}
				logPresenter.FPrint(p, alloc.RecentLogs)
				p.Flush()
			}

		}
		return nil
	}
	monitor.DeploymentSucceeded = func(d *api.DeploymentStatus) error {
		fmt.Fprintf(cc.Out, "v%d deployed successfully\n", d.Version)
		return nil
	}

	monitor.Start(ctx)

	if err := monitor.Error(); err != nil {
		fmt.Fprintf(cc.Out, "Monitor Error: %s", err)
	}

	if !monitor.Success() {
		return ErrAbort
	}

	return nil
}
