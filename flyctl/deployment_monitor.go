package flyctl

import (
	"fmt"
	"io"
	"reflect"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/morikuni/aec"
	"github.com/superfly/flyctl/api"
)

func NewDeploymentMonitor(client *api.Client, appID string) *DeploymentMonitor {
	return &DeploymentMonitor{
		AppID:  appID,
		client: client,
	}
}

type DeploymentMonitor struct {
	AppID string

	client       *api.Client
	err          error
	successCount int
	failureCount int
}

var pollInterval = 750 * time.Millisecond

func (dm *DeploymentMonitor) Start() <-chan *DeploymentStatus {
	statusCh := make(chan *DeploymentStatus)

	go func() {
		defer close(statusCh)

		var currentDeployment *DeploymentStatus
		currentID := ""
		prevID := ""
		num := 0
		startTime := time.Now()

		for {
			deployment, err := dm.client.GetDeploymentStatus(dm.AppID, currentID)
			if err != nil {
				fmt.Println("got err", err)
				dm.err = err
				break
			}

			if deployment == nil {
				if time.Now().After(startTime.Add(5 * time.Second)) {
					fmt.Println("No deployment available")
					// nothing to show after 5 seconds, break
					break
				}
				time.Sleep(pollInterval)
				continue
			}

			if deployment.ID == prevID {
				// deployment already done, bail
				break
			}

			if currentDeployment == nil && !deployment.InProgress {
				// wait for deployment (new deployment not yet created)
				continue
			}

			// this is a new deployment
			if currentDeployment == nil {
				currentDeployment = newDeploymentStatus(deployment)
				num++
				currentDeployment.number = num
				statusCh <- currentDeployment
				currentID = deployment.ID
			}

			currentDeployment.Update(deployment)

			if !deployment.InProgress && currentDeployment != nil {
				// deployment is complete, close out and reset for next iteration
				currentDeployment.Close()
				if deployment.Successful {
					dm.successCount++
				} else {
					dm.failureCount++
				}
				currentDeployment = nil
				prevID = currentID
				currentID = ""
			}

			time.Sleep(pollInterval)
		}
	}()

	return statusCh
}

func (dm *DeploymentMonitor) Success() bool {
	return dm.failureCount == 0
}

func (dm *DeploymentMonitor) Failed() bool {
	return dm.failureCount > 0
}

func (dm *DeploymentMonitor) Error() error {
	return dm.err
}

func (dm *DeploymentMonitor) DisplayVerbose(w io.Writer) {
	for deployment := range dm.Start() {
		if deployment.number > 1 {
			fmt.Fprintln(w)
		}

		fmt.Fprintln(w, deployment.DeploymentSummary())

		for updatedAllocs := range deployment.update {
			for _, alloc := range updatedAllocs {
				fmt.Fprintln(w, formatAllocSummary(alloc))
			}
		}

		deployment.printFailingAllocs(w)
		fmt.Fprintln(w, deployment.DeploymentSummary())
	}
}

func (dm *DeploymentMonitor) DisplayCompact(w io.Writer) {
	for deployment := range dm.Start() {
		if deployment.number > 1 {
			fmt.Fprintln(w)
		}

		fmt.Fprintln(w, deployment.DeploymentSummary())
		fmt.Fprintln(w, deployment.AllocSummary())

		for range deployment.update {
			fmt.Fprint(w, aec.Up(1))
			fmt.Fprint(w, aec.EraseLine(aec.EraseModes.All))
			fmt.Fprintln(w, deployment.AllocSummary())
		}

		deployment.printFailingAllocs(w)
		fmt.Fprintln(w, deployment.DeploymentSummary())
	}
}

type DeploymentStatus struct {
	number      int
	deployment  *api.DeploymentStatus
	allocStatus map[string]api.AllocationStatus
	allocLogs   map[string]*allocLog
	update      chan []api.AllocationStatus
}

func newDeploymentStatus(deployment *api.DeploymentStatus) *DeploymentStatus {
	return &DeploymentStatus{
		deployment:  deployment,
		allocStatus: map[string]api.AllocationStatus{},
		allocLogs:   map[string]*allocLog{},
		update:      make(chan []api.AllocationStatus),
	}
}

func (ds *DeploymentStatus) Update(updatedDeployment *api.DeploymentStatus) {
	if reflect.DeepEqual(ds.deployment, updatedDeployment) {
		return
	}

	// deployment data has changed, cache & forward the updates

	ds.deployment = updatedDeployment

	updatedAllocs := []api.AllocationStatus{}
	for _, aNew := range updatedDeployment.Allocations {
		aPrev, ok := ds.allocStatus[aNew.ID]
		if ok && reflect.DeepEqual(aNew, aPrev) {
			continue
		}
		ds.allocStatus[aNew.ID] = aNew
		updatedAllocs = append(updatedAllocs, aNew)

		log, ok := ds.allocLogs[aNew.ID]
		if !ok {
			log = &allocLog{
				events: map[api.AllocationEvent]bool{},
				checks: map[string]api.CheckState{},
			}
			ds.allocLogs[aNew.ID] = log
		}
		log.Append(aNew)
	}

	ds.update <- updatedAllocs
}

func (ds *DeploymentStatus) Close() {
	close(ds.update)
}

func (dm *DeploymentStatus) FailingAllocs() []api.AllocationStatus {
	out := []api.AllocationStatus{}
	for _, alloc := range dm.allocStatus {
		if !alloc.Healthy {
			log := dm.allocLogs[alloc.ID]
			alloc.Events = log.Events()
			alloc.Checks = log.FailingChecks()
			out = append(out, alloc)
		}
	}
	return out
}

func (ds *DeploymentStatus) DeploymentSummary() string {
	if ds.deployment.InProgress {
		return fmt.Sprintf("v%d is being deployed", ds.deployment.Version)
	}
	if ds.deployment.Successful {
		return fmt.Sprintf("v%d deployed successfully", ds.deployment.Version)
	}

	return fmt.Sprintf("v%d %s - %s", ds.deployment.Version, ds.deployment.Status, ds.deployment.Description)
}

func (ds *DeploymentStatus) AllocSummary() string {
	allocCounts := fmt.Sprintf("%d desired, %d placed, %d healthy, %d unhealthy", ds.deployment.DesiredCount,
		ds.deployment.PlacedCount, ds.deployment.HealthyCount, ds.deployment.UnhealthyCount)

	checkCounts := formatHealthChecksSummary(ds.deployment.Allocations...)

	if checkCounts == "" {
		return allocCounts
	}

	return allocCounts + " [" + checkCounts + "]"
}

func (ds *DeploymentStatus) printFailingAllocs(w io.Writer) {
	failingAllocs := ds.FailingAllocs()
	if len(failingAllocs) > 0 {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "Failed Allocations")

		for idx, alloc := range failingAllocs {
			canaryMsg := ""
			if alloc.Canary {
				canaryMsg = " [canary]"
			}
			fmt.Fprintf(w, "  %d) %s in %s%s\n", idx+1, alloc.IDShort, alloc.Region, canaryMsg)

			if len(alloc.Events) > 0 {
				fmt.Fprintf(w, "    Events\n")
				tw := tabwriter.NewWriter(w, 0, 0, 3, ' ', tabwriter.TabIndent)
				for _, event := range alloc.Events {
					fmt.Fprintf(tw, "      %s\t%s\t%s\n", event.Timestamp.Format(time.RFC3339), event.Type, event.Message)
				}
				tw.Flush()
			}

			if len(alloc.Checks) > 0 {
				fmt.Fprintf(w, "    Checks\n")
				for _, check := range alloc.Checks {
					fmt.Fprintf(w, "      Check %s: %s\n", check.Name, check.Status)
					for _, line := range splitLines(check.Output) {
						fmt.Fprintf(w, "        %s\n", line)
					}
				}
			}
		}
		fmt.Fprintln(w)
	}
}

type allocLog struct {
	events map[api.AllocationEvent]bool
	checks map[string]api.CheckState
}

func (l *allocLog) Append(alloc api.AllocationStatus) {
	for _, event := range alloc.Events {
		l.events[event] = true
	}

	for _, check := range alloc.Checks {
		l.checks[check.Name] = check
	}
}

func (l *allocLog) Events() []api.AllocationEvent {
	out := []api.AllocationEvent{}
	for e := range l.events {
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Timestamp.Before(out[j].Timestamp) })
	return out
}

func (l *allocLog) FailingChecks() []api.CheckState {
	out := []api.CheckState{}
	for _, c := range l.checks {
		if c.Status != "passing" {
			out = append(out, c)
		}
	}
	return out
}

func splitLines(s string) []string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.TrimSpace(s)
	return strings.Split(s, "\n")
}

func passingChecks(checks []api.CheckState) (n int) {
	for _, check := range checks {
		if check.Status == "passing" {
			n++
		}
	}
	return n
}

func warnChecks(checks []api.CheckState) (n int) {
	for _, check := range checks {
		if check.Status == "warn" {
			n++
		}
	}
	return n
}

func critChecks(checks []api.CheckState) (n int) {
	for _, check := range checks {
		if check.Status == "critical" {
			n++
		}
	}
	return n
}

func formatAllocSummary(alloc api.AllocationStatus) string {
	msg := fmt.Sprintf("%s: %s %s", alloc.IDShort, alloc.Region, alloc.Status)

	if alloc.Status == "pending" {
		return msg
	}

	if alloc.Failed {
		msg += " failed"
	} else if alloc.Healthy {
		msg += " healthy"
	} else {
		msg += " unhealthy"
	}

	if alloc.Canary {
		msg += " [canary]"
	}

	if checkStr := formatHealthChecksSummary(alloc); checkStr != "" {
		msg += " [" + checkStr + "]"
	}

	return msg
}

func formatHealthChecksSummary(allocs ...api.AllocationStatus) string {
	var total, pass, crit, warn int

	for _, alloc := range allocs {
		if n := len(alloc.Checks); n > 0 {
			total += n
			pass += passingChecks(alloc.Checks)
			crit += critChecks(alloc.Checks)
			warn += warnChecks(alloc.Checks)
		}
	}

	if total == 0 {
		return ""
	}

	checkStr := fmt.Sprintf("%d total", total)

	if pass > 0 {
		checkStr += ", " + fmt.Sprintf("%d passing", pass)
	}
	if warn > 0 {
		checkStr += ", " + fmt.Sprintf("%d warning", warn)
	}
	if crit > 0 {
		checkStr += ", " + fmt.Sprintf("%d critical", crit)
	}

	return "health checks: " + checkStr
}
