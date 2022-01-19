package watch

import (
	"context"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/briandowns/spinner"
	"golang.org/x/sync/errgroup"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/cmd/presenters"
	"github.com/superfly/flyctl/pkg/iostreams"
	"github.com/superfly/flyctl/pkg/logs"

	"github.com/superfly/flyctl/internal/cli/internal/app"
	"github.com/superfly/flyctl/internal/cli/internal/config"
	"github.com/superfly/flyctl/internal/cli/internal/format"
	"github.com/superfly/flyctl/internal/cli/internal/render"
	"github.com/superfly/flyctl/internal/client"
	"github.com/superfly/flyctl/internal/deployment"
	"github.com/superfly/flyctl/internal/flyerr"
)

func WatchDeployment(ctx context.Context) error {
	tb := render.NewTextBlock(ctx, "Monitoring deployment")

	io := iostreams.FromContext(ctx)
	appName := app.NameFromContext(ctx)
	client := client.FromContext(ctx).API()
	endmessage := ""

	monitor := deployment.NewDeploymentMonitor(client, appName)

	monitor.DeploymentStarted = func(idx int, d *api.DeploymentStatus) error {
		if idx > 0 {
			tb.Println()
		}
		tb.Println(format.DeploymentSummary(d))

		return nil
	}

	// TODO check we aren't asking for JSON
	monitor.DeploymentUpdated = func(d *api.DeploymentStatus, updatedAllocs []*api.AllocationStatus) error {
		if io.IsInteractive() {
			tb.Overwrite()

			tb.Println(format.DeploymentAllocSummary(d))
		} else {
			for _, alloc := range updatedAllocs {
				tb.Println(format.AllocSummary(alloc))
			}
		}

		return nil
	}

	monitor.DeploymentFailed = func(d *api.DeploymentStatus, failedAllocs []*api.AllocationStatus) error {
		// cmdCtx.Statusf("deploy", cmdctx.SDETAIL, "v%d %s - %s\n", d.Version, d.Status, d.Description)

		if endmessage == "" && d.Status == "failed" {
			if strings.Contains(d.Description, "no stable release to revert to") {
				endmessage = fmt.Sprintf("v%d %s - %s\n", d.Version, d.Status, d.Description)
			} else {
				endmessage = fmt.Sprintf("v%d %s - %s and deploying as v%d \n", d.Version, d.Status, d.Description, d.Version+1)
			}
		}

		if len(failedAllocs) > 0 {
			tb.Println("Failed Instances")

			x := make(chan *api.AllocationStatus)
			var wg sync.WaitGroup
			wg.Add(len(failedAllocs))

			for _, a := range failedAllocs {
				a := a
				go func() {
					defer wg.Done()
					alloc, err := client.GetAllocationStatus(ctx, appName, a.ID, 30)
					if err != nil {
						//cmdCtx.Status("deploy", cmdctx.SERROR, "Error fetching alloc", a.ID, err)
						tb.Printf("failed fetching alloc %s: %s", a.ID, err)

						return
					}
					x <- alloc
				}()
			}

			go func() {
				wg.Wait()
				close(x)
			}()

			var count int
			for alloc := range x {
				count++

				tb.Println()
				tb.Printf("Failure #%d\n", count)
				tb.Println()

				if err := render.AllocationStatuses(io.Out, "Instance", []api.Region{}, alloc); err != nil {
					return fmt.Errorf("failed rendering alloc status: %w", err)
				}

				if err := render.AllocationEvents(io.Out, "Recent Events", alloc.Events...); err != nil {
					return fmt.Errorf("failed rendering recent events: %w", err)
				}

				renderLogs(ctx, alloc)
			}
		}

		return nil
	}

	monitor.DeploymentSucceeded = func(d *api.DeploymentStatus) error {
		tb.Donef("v%d deployed successfully\n", d.Version)

		return nil
	}

	monitor.Start(ctx)

	if err := monitor.Error(); err != nil {
		return err
	}

	if endmessage != "" {
		tb.Done(endmessage)
	}

	if !monitor.Success() {
		tb.Done("Troubleshooting guide at https://fly.io/docs/getting-started/troubleshooting/")
		return flyerr.ErrAbort
	}

	return nil
}

func WatchReleaseCommand(ctx context.Context, id string) error {
	g, ctx := errgroup.WithContext(ctx)
	io := iostreams.FromContext(ctx)
	client := client.FromContext(ctx).API()
	interactive := io.IsInteractive()
	appName := app.NameFromContext(ctx)

	s := spinner.New(spinner.CharSets[11], 100*time.Millisecond)
	s.Writer = os.Stderr
	s.Prefix = "Running release task..."

	if interactive {
		s.Start()
		defer s.Stop()
	}

	rcUpdates := make(chan api.ReleaseCommand)

	startLogs := func(ctx context.Context, vmid string) {
		g.Go(func() error {
			childCtx, cancel := context.WithCancel(ctx)
			defer cancel()

			opts := &logs.LogOptions{
				MaxBackoff: time.Second,
				AppName:    appName,
				VMID:       vmid,
			}

			ls, err := logs.NewPollingStream(childCtx, client, opts)
			if err != nil {
				return err
			}

			for entry := range ls.Stream(childCtx, opts) {
				if interactive {
					s.Stop()
				}

				fmt.Fprintln(io.Out, "\t", entry.Message)

				// watch for the shutdown message
				if entry.Message == "Starting clean up." {
					cancel()
				}
				s.Start()
			}

			if err = ls.Err(); errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				err = nil
			}
			return err
		})
	}

	g.Go(func() error {
		var lastValue *api.ReleaseCommand
		var errorCount int
		defer close(rcUpdates)

		for {
			rc, err := client.GetReleaseCommand(ctx, id)
			if err != nil {
				errorCount += 1
				if errorCount < 3 {
					continue
				}
				return err
			}

			if !reflect.DeepEqual(lastValue, rc) {
				lastValue = rc
				rcUpdates <- *rc
			}

			if !rc.InProgress {
				break
			}

			time.Sleep(500 * time.Millisecond)
		}

		return nil
	})

	g.Go(func() error {
		// The logs goroutine will stop itself when it sees a shutdown log message.
		// If the message never comes (delayed logs, etc) the deploy will hang.
		// This timeout makes sure they always stop a few seconds after the release task is done.
		logsCtx, logsCancel := context.WithCancel(ctx)
		defer time.AfterFunc(3*time.Second, logsCancel)

		for rc := range rcUpdates {
			if interactive {
				s.Prefix = fmt.Sprintf("Running release task (%s)...", rc.Status)
			}

			if rc.InstanceID != nil {
				startLogs(logsCtx, *rc.InstanceID)
			}

			if !rc.InProgress && rc.Failed {
				if rc.Succeeded && interactive {
					s.FinalMSG = "Running release task...Done\n"
				} else if rc.Failed {
					return fmt.Errorf("release command failed, deployment aborted")
				}
			}
		}

		return nil
	})

	return g.Wait()
}

func renderLogs(ctx context.Context, alloc *api.AllocationStatus) {
	out := iostreams.FromContext(ctx).Out
	cfg := config.FromContext(ctx).JSONOutput

	logPresenter := presenters.LogPresenter{
		HideAllocID:    true,
		HideRegion:     true,
		RemoveNewlines: true,
	}

	for _, e := range alloc.RecentLogs {
		entry := logs.LogEntry{
			Instance:  e.Instance,
			Level:     e.Level,
			Message:   e.Message,
			Region:    e.Region,
			Timestamp: e.Timestamp,
			Meta:      e.Meta,
		}

		logPresenter.FPrint(out, cfg, entry)
	}
}
