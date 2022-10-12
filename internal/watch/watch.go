package watch

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/morikuni/aec"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/flaps"
	"github.com/superfly/flyctl/iostreams"
	"github.com/superfly/flyctl/logs"

	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/internal/app"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/deployment"
	"github.com/superfly/flyctl/internal/flyerr"
	"github.com/superfly/flyctl/internal/format"
	"github.com/superfly/flyctl/internal/render"
	"github.com/superfly/flyctl/internal/spinner"
)

func Deployment(ctx context.Context, appName, evaluationID string) error {
	tb := render.NewTextBlock(ctx, "Monitoring deployment")

	io := iostreams.FromContext(ctx)
	client := client.FromContext(ctx).API()
	endmessage := ""

	monitor := deployment.NewDeploymentMonitor(client, appName, evaluationID)

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

func ReleaseCommand(ctx context.Context, id string) error {
	g, ctx := errgroup.WithContext(ctx)
	io := iostreams.FromContext(ctx)
	client := client.FromContext(ctx).API()
	interactive := io.IsInteractive()
	appName := app.NameFromContext(ctx)

	s := spinner.Run(io, "Running release task ...")
	defer s.Stop()

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

			ls, err := logs.NewPollingStream(client, opts)
			if err != nil {
				return err
			}

			for entry := range ls.Stream(childCtx, opts) {
				msg := s.Stop()

				fmt.Fprintln(io.Out, "\t", entry.Message)

				// watch for the shutdown message
				if entry.Message == "Starting clean up." {
					cancel()
				}

				s.StartWithMessage(msg)
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
			rc, err := func() (*api.ReleaseCommand, error) {
				reqCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
				defer cancel()
				rc, err := client.GetReleaseCommand(reqCtx, id)
				if ctxErr := reqCtx.Err(); ctxErr != nil {
					return nil, ctxErr
				}
				return rc, err
			}()
			if err != nil {
				if err == context.DeadlineExceeded {
					// don't increment error count if this is a timeout
					continue
				}
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
			msg := fmt.Sprintf("Running release task (%s)...", rc.Status)
			s.Set(msg)

			if rc.InstanceID != nil {
				startLogs(logsCtx, *rc.InstanceID)
			}

			if !rc.InProgress && rc.Failed {
				if rc.Succeeded && interactive {
					s.StopWithMessage("Running release task... Done.")
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
	cfg := config.FromContext(ctx)

	for _, e := range alloc.RecentLogs {
		entry := logs.LogEntry{
			Instance:  e.Instance,
			Level:     e.Level,
			Message:   e.Message,
			Region:    e.Region,
			Timestamp: e.Timestamp,
			Meta:      e.Meta,
		}

		if cfg.JSONOutput {
			render.JSON(out, entry)
		}

		render.LogEntry(
			out,
			entry,
			render.HideAllocID(),
			render.HideRegion(),
		)
	}
}

func MachinesChecks(ctx context.Context, machines []*api.Machine) (err error) {
	var (
		io          = iostreams.FromContext(ctx)
		colorize    = io.ColorScheme()
		flapsClient = flaps.FromContext(ctx)
	)
	ctx, cancel := context.WithTimeout(ctx, 300*time.Second)
	defer cancel()

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	var iterations int
	var errCount int

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			var allChecks []*api.MachineCheckStatus
			var checked []*api.Machine

			err = func() (err error) {
				for _, machine := range machines {
					machine, err = flapsClient.Get(ctx, machine.ID)
					if err != nil {
						return
					}
					checked = append(checked, machine)

				}
				return
			}()
			if err != nil {
				errCount++
				if errCount > 6 {
					return
				}
				continue
			}

			iterations++

			if io.IsInteractive() && iterations > 1 {
				builder := aec.EmptyBuilder
				str := builder.Up(uint(len(checked))).EraseLine(aec.EraseModes.All).ANSI
				fmt.Fprint(io.ErrOut, str.String())
			}

			for _, machine := range checked {
				if machine.Config.Checks == nil {
					continue
				}

				allChecks = append(allChecks, machine.Checks...)

				var pass, _, _ = countChecks(machine.Checks)

				// Waiting for xxxxxxxx to become healthy (started, 3/3)
				fmt.Fprintf(io.ErrOut, "  Waiting for %s to become healthy (%s, %s)\n",
					colorize.Bold(machine.ID),
					colorize.Green(machine.State),
					colorize.Green(fmt.Sprintf("%d/%d", pass, len(machine.Checks))),
				)
			}

			if len(allChecks) == 0 {
				fmt.Fprintln(io.Out)

				fmt.Fprintln(io.Out, "No health checks found")
				return
			}
			var pass, _, _ = countChecks(allChecks)

			// if all checks are passing, we're done
			if pass == len(allChecks) {
				return
			}
		}
	}
}

func countChecks(checks []*api.MachineCheckStatus) (pass, warn, crit int) {
	for _, check := range checks {
		switch check.Status {
		case "passing":
			pass++
		case "warn":
			warn++
		case "critical":
			crit++
		}
	}
	return pass, warn, crit
}
