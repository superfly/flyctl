package deployment

import (
	"context"
	"errors"
	"reflect"
	"time"

	"github.com/hashicorp/go-multierror"
	"github.com/superfly/flyctl/api"
)

var ErrNoDeployment = errors.New("No deployment available to monitor")
var errDeploymentNotReady = errors.New("Deployment not ready to monitor")
var errDeploymentComplete = errors.New("Deployment is already complete")

func NewDeploymentMonitor(appID string) *DeploymentMonitor {
	return &DeploymentMonitor{
		AppID: appID,
	}
}

type DeploymentMonitor struct {
	AppID string

	client       *api.Client
	err          error
	successCount int
	failureCount int

	DeploymentStarted   func(idx int, deployment *api.DeploymentStatus) error
	DeploymentUpdated   func(deployment *api.DeploymentStatus, updatedAllocs []*api.AllocationStatus) error
	DeploymentFailed    func(deployment *api.DeploymentStatus, failedAllocs []*api.AllocationStatus) error
	DeploymentSucceeded func(deployment *api.DeploymentStatus) error
}

var pollInterval = 750 * time.Millisecond

func (dm *DeploymentMonitor) start(ctx context.Context) <-chan *deploymentStatus {
	statusCh := make(chan *deploymentStatus)

	go func() {
		var currentDeployment *deploymentStatus

		defer func() {
			if currentDeployment != nil {
				currentDeployment.Close()
			}

			close(statusCh)
		}()

		currentID := ""
		prevID := ""
		num := 0
		startTime := time.Now()

		var delay time.Duration

		processFn := func() error {
			deployment, err := dm.client.GetDeploymentStatus(ctx, dm.AppID, currentID)
			if err != nil {
				return err
			}

			// wait for a deployment for up to 30 seconds. Could be due to a delay submitting the job or because
			// there is no active deployment
			if deployment == nil {
				if time.Now().After(startTime.Add(5 * time.Minute)) {
					// nothing to show after 5 minutes, break
					return ErrNoDeployment
				}
				return errDeploymentNotReady
			}

			if deployment.ID == prevID {
				// deployment already done, bail
				return errDeploymentComplete
			}

			if currentDeployment == nil && !deployment.InProgress {
				// wait for deployment (new deployment not yet created)
				return errDeploymentNotReady
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

			return nil
		}

		for {
			select {
			case <-time.After(delay):
				switch err := processFn(); err {
				case nil:
					// we're still monitoring, ensure the poll interval is > 0 and continue
					delay = pollInterval
				case errDeploymentComplete:
					// we're done, exit
					return
				case errDeploymentNotReady:
					// we're waiting for a deployment, set the poll interval to a small value and continue
					delay = pollInterval / 2
				default:
					dm.err = multierror.Append(err)
					return
				}
			case <-ctx.Done():
				if ctx.Err() != nil {
					dm.err = multierror.Append(dm.err, ctx.Err())
				}
				return
			}
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

func (dm *DeploymentMonitor) Start(ctx context.Context) {
	for deployment := range dm.start(ctx) {
		if dm.DeploymentStarted != nil {
			if err := dm.DeploymentStarted(deployment.number, deployment.deployment); err != nil {
				dm.err = multierror.Append(dm.err, err)
				return
			}
		}

		for updatedAllocs := range deployment.update {
			if dm.DeploymentUpdated != nil {
				if err := dm.DeploymentUpdated(deployment.deployment, updatedAllocs); err != nil {
					dm.err = multierror.Append(dm.err, err)
					return
				}
			}
		}

		if deployment.deployment.Successful {
			if dm.DeploymentSucceeded != nil {
				if err := dm.DeploymentSucceeded(deployment.deployment); err != nil {
					dm.err = multierror.Append(dm.err, err)
					return
				}
			}
		} else {
			if dm.DeploymentFailed != nil {
				if err := dm.DeploymentFailed(deployment.deployment, deployment.FailingAllocs()); err != nil {
					dm.err = multierror.Append(dm.err, err)
					return
				}
			}
		}
	}
}

type deploymentStatus struct {
	number      int
	deployment  *api.DeploymentStatus
	allocStatus map[string]*api.AllocationStatus
	update      chan []*api.AllocationStatus
}

func newDeploymentStatus(deployment *api.DeploymentStatus) *deploymentStatus {
	return &deploymentStatus{
		deployment:  deployment,
		allocStatus: map[string]*api.AllocationStatus{},
		update:      make(chan []*api.AllocationStatus),
	}
}

func (ds *deploymentStatus) Update(updatedDeployment *api.DeploymentStatus) {
	if reflect.DeepEqual(ds.deployment, updatedDeployment) {
		return
	}

	// deployment data has changed, cache & forward the updates

	ds.deployment = updatedDeployment

	updatedAllocs := []*api.AllocationStatus{}
	for _, aNew := range updatedDeployment.Allocations {
		aPrev, ok := ds.allocStatus[aNew.ID]
		if ok && reflect.DeepEqual(aNew, aPrev) {
			continue
		}
		ds.allocStatus[aNew.ID] = aNew
		updatedAllocs = append(updatedAllocs, aNew)
	}

	ds.update <- updatedAllocs
}

func (ds *deploymentStatus) Close() {
	close(ds.update)
}

func (dm *deploymentStatus) FailingAllocs() []*api.AllocationStatus {
	out := []*api.AllocationStatus{}
	for _, alloc := range dm.allocStatus {
		if !alloc.Healthy || alloc.Status == "failed" {
			out = append(out, alloc)
		}
	}
	return out
}
