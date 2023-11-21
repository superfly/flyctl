package flaps

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/google/go-querystring/query"
	"github.com/samber/lo"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/internal/metrics"
	"github.com/superfly/flyctl/terminal"
)

var NonceHeader = "fly-machine-lease-nonce"

func (f *Client) sendRequestMachines(ctx context.Context, action flapsAction, method, endpoint string, in, out interface{}, headers map[string][]string) error {
	endpoint = fmt.Sprintf("/apps/%s/machines%s", f.appName, endpoint)
	return f._sendRequest(ctx, action, method, endpoint, in, out, headers)
}

func (f *Client) Launch(ctx context.Context, builder api.LaunchMachineInput) (out *api.Machine, err error) {
	metrics.Started(ctx, "machine_launch")
	sendUpdateMetrics := metrics.StartTiming(ctx, "machine_launch/duration")
	defer func() {
		metrics.Status(ctx, "machine_launch", err == nil)
		if err == nil {
			sendUpdateMetrics()
		}
	}()

	out = new(api.Machine)
	if err := f.sendRequestMachines(ctx, machineLaunch, http.MethodPost, "", builder, out, nil); err != nil {
		return nil, fmt.Errorf("failed to launch VM: %w", err)
	}

	return out, nil
}

func (f *Client) Update(ctx context.Context, builder api.LaunchMachineInput, nonce string) (out *api.Machine, err error) {
	headers := make(map[string][]string)
	if nonce != "" {
		headers[NonceHeader] = []string{nonce}
	}

	metrics.Started(ctx, "machine_update")
	sendUpdateMetrics := metrics.StartTiming(ctx, "machine_update/duration")
	defer func() {
		metrics.Status(ctx, "machine_update", err == nil)
		if err == nil {
			sendUpdateMetrics()
		}
	}()

	endpoint := fmt.Sprintf("/%s", builder.ID)
	out = new(api.Machine)
	if err := f.sendRequestMachines(ctx, machineUpdate, http.MethodPost, endpoint, builder, out, headers); err != nil {
		return nil, fmt.Errorf("failed to update VM %s: %w", builder.ID, err)
	}
	return out, nil
}

func (f *Client) Start(ctx context.Context, machineID string, nonce string) (out *api.MachineStartResponse, err error) {
	startEndpoint := fmt.Sprintf("/%s/start", machineID)

	headers := make(map[string][]string)
	if nonce != "" {
		headers[NonceHeader] = []string{nonce}
	}

	out = new(api.MachineStartResponse)

	metrics.Started(ctx, "machine_start")
	defer func() {
		metrics.Status(ctx, "machine_start", err == nil)
	}()

	if err := f.sendRequestMachines(ctx, machineStart, http.MethodPost, startEndpoint, nil, out, headers); err != nil {
		return nil, fmt.Errorf("failed to start VM %s: %w", machineID, err)
	}
	return out, nil
}

type waitQuerystring struct {
	InstanceId     string `url:"instance_id,omitempty"`
	TimeoutSeconds int    `url:"timeout,omitempty"`
	State          string `url:"state,omitempty"`
}

const proxyTimeoutThreshold = 60 * time.Second

func (f *Client) Wait(ctx context.Context, machine *api.Machine, state string, timeout time.Duration) (err error) {
	waitEndpoint := fmt.Sprintf("/%s/wait", machine.ID)
	if state == "" {
		state = "started"
	}
	version := machine.InstanceID
	if machine.Version != "" {
		version = machine.Version
	}
	if timeout > proxyTimeoutThreshold {
		timeout = proxyTimeoutThreshold
	}
	if timeout < 1*time.Second {
		timeout = 1 * time.Second
	}
	timeoutSeconds := int(timeout.Seconds())
	waitQs := waitQuerystring{
		InstanceId:     version,
		TimeoutSeconds: timeoutSeconds,
		State:          state,
	}
	qsVals, err := query.Values(waitQs)
	if err != nil {
		return fmt.Errorf("error making query string for wait request: %w", err)
	}
	waitEndpoint += fmt.Sprintf("?%s", qsVals.Encode())
	if err := f.sendRequestMachines(ctx, machineWait, http.MethodGet, waitEndpoint, nil, nil, nil); err != nil {
		return fmt.Errorf("failed to wait for VM %s in %s state: %w", machine.ID, state, err)
	}
	return
}

func (f *Client) Stop(ctx context.Context, in api.StopMachineInput, nonce string) (err error) {
	stopEndpoint := fmt.Sprintf("/%s/stop", in.ID)

	headers := make(map[string][]string)
	if nonce != "" {
		headers[NonceHeader] = []string{nonce}
	}

	if err := f.sendRequestMachines(ctx, machineStop, http.MethodPost, stopEndpoint, in, nil, headers); err != nil {
		return fmt.Errorf("failed to stop VM %s: %w", in.ID, err)
	}
	return
}

func (f *Client) Restart(ctx context.Context, in api.RestartMachineInput, nonce string) (err error) {
	headers := make(map[string][]string)
	if nonce != "" {
		headers[NonceHeader] = []string{nonce}
	}

	restartEndpoint := fmt.Sprintf("/%s/restart?force_stop=%t", in.ID, in.ForceStop)

	if in.Timeout != 0 {
		restartEndpoint += fmt.Sprintf("&timeout=%d", in.Timeout)
	}

	if in.Signal != "" {
		restartEndpoint += fmt.Sprintf("&signal=%s", in.Signal)
	}

	if err := f.sendRequestMachines(ctx, machineRestart, http.MethodPost, restartEndpoint, nil, nil, headers); err != nil {
		return fmt.Errorf("failed to restart VM %s: %w", in.ID, err)
	}
	return
}

func (f *Client) Get(ctx context.Context, machineID string) (*api.Machine, error) {
	getEndpoint := ""

	if machineID != "" {
		getEndpoint = fmt.Sprintf("/%s", machineID)
	}

	out := new(api.Machine)

	err := f.sendRequestMachines(ctx, machineGet, http.MethodGet, getEndpoint, nil, out, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get VM %s: %w", machineID, err)
	}
	return out, nil
}

func (f *Client) GetMany(ctx context.Context, machineIDs []string) ([]*api.Machine, error) {
	machines := make([]*api.Machine, 0, len(machineIDs))
	for _, id := range machineIDs {
		m, err := f.Get(ctx, id)
		if err != nil {
			return machines, err
		}
		machines = append(machines, m)
	}
	return machines, nil
}

func (f *Client) List(ctx context.Context, state string) ([]*api.Machine, error) {
	getEndpoint := ""

	if state != "" {
		getEndpoint = fmt.Sprintf("?%s", state)
	}

	out := make([]*api.Machine, 0)

	err := f.sendRequestMachines(ctx, machineList, http.MethodGet, getEndpoint, nil, &out, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list VMs: %w", err)
	}
	return out, nil
}

// ListActive returns only non-destroyed that aren't in a reserved process group.
func (f *Client) ListActive(ctx context.Context) ([]*api.Machine, error) {
	getEndpoint := ""

	machines := make([]*api.Machine, 0)

	err := f.sendRequestMachines(ctx, machineList, http.MethodGet, getEndpoint, nil, &machines, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list active VMs: %w", err)
	}

	machines = lo.Filter(machines, func(m *api.Machine, _ int) bool {
		return !m.IsReleaseCommandMachine() && !m.IsFlyAppsConsole() && m.IsActive()
	})

	return machines, nil
}

// ListFlyAppsMachines returns apps that are part of the Fly apps platform.
// Destroyed machines and console machines are excluded.
// Unlike other List functions, this function retries multiple times.
func (f *Client) ListFlyAppsMachines(ctx context.Context) ([]*api.Machine, *api.Machine, error) {
	var allMachines []*api.Machine
	b := backoff.NewExponentialBackOff()
	b.InitialInterval = 500 * time.Millisecond
	b.MaxElapsedTime = 5 * time.Second
	err := backoff.Retry(func() error {
		err := f.sendRequestMachines(ctx, machineList, http.MethodGet, "", nil, &allMachines, nil)
		if err != nil {
			if errors.Is(err, FlapsErrorNotFound) {
				return err
			} else {
				return backoff.Permanent(err)
			}
		}
		return nil
	}, backoff.WithContext(b, ctx))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to list VMs even after retries: %w", err)
	}
	var releaseCmdMachine *api.Machine
	machines := make([]*api.Machine, 0)
	for _, m := range allMachines {
		if m.IsFlyAppsPlatform() && m.IsActive() && !m.IsFlyAppsReleaseCommand() && !m.IsFlyAppsConsole() {
			machines = append(machines, m)
		} else if m.IsFlyAppsReleaseCommand() {
			releaseCmdMachine = m
		}
	}
	return machines, releaseCmdMachine, nil
}

func (f *Client) Destroy(ctx context.Context, input api.RemoveMachineInput, nonce string) (err error) {
	headers := make(map[string][]string)
	if nonce != "" {
		headers[NonceHeader] = []string{nonce}
	}

	destroyEndpoint := fmt.Sprintf("/%s?kill=%t", input.ID, input.Kill)

	if err := f.sendRequestMachines(ctx, machineDestroy, http.MethodDelete, destroyEndpoint, nil, nil, headers); err != nil {
		return fmt.Errorf("failed to destroy VM %s: %w", input.ID, err)
	}

	return
}

func (f *Client) Kill(ctx context.Context, machineID string) (err error) {
	in := map[string]interface{}{
		"signal": 9,
	}
	err = f.sendRequestMachines(ctx, machineKill, http.MethodPost, fmt.Sprintf("/%s/signal", machineID), in, nil, nil)

	if err != nil {
		return fmt.Errorf("failed to kill VM %s: %w", machineID, err)
	}
	return
}

func (f *Client) FindLease(ctx context.Context, machineID string) (*api.MachineLease, error) {
	endpoint := fmt.Sprintf("/%s/lease", machineID)

	out := new(api.MachineLease)

	err := f.sendRequestMachines(ctx, machineFindLease, http.MethodGet, endpoint, nil, out, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get lease on VM %s: %w", machineID, err)
	}
	return out, nil
}

func (f *Client) AcquireLease(ctx context.Context, machineID string, ttl *int) (*api.MachineLease, error) {
	endpoint := fmt.Sprintf("/%s/lease", machineID)

	if ttl != nil {
		endpoint += fmt.Sprintf("?ttl=%d", *ttl)
	}

	out := new(api.MachineLease)

	err := f.sendRequestMachines(ctx, machineAcquireLease, http.MethodPost, endpoint, nil, out, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get lease on VM %s: %w", machineID, err)
	}
	terminal.Debugf("got lease on machine %s: %v\n", machineID, out)
	return out, nil
}

func (f *Client) RefreshLease(ctx context.Context, machineID string, ttl *int, nonce string) (*api.MachineLease, error) {
	endpoint := fmt.Sprintf("/%s/lease", machineID)
	if ttl != nil {
		endpoint += fmt.Sprintf("?ttl=%d", *ttl)
	}
	headers := make(map[string][]string)
	headers[NonceHeader] = []string{nonce}
	out := new(api.MachineLease)
	err := f.sendRequestMachines(ctx, machineRefreshLease, http.MethodPost, endpoint, nil, out, headers)
	if err != nil {
		return nil, fmt.Errorf("failed to get lease on VM %s: %w", machineID, err)
	}
	terminal.Debugf("got lease on machine %s: %v\n", machineID, out)
	return out, nil
}

func (f *Client) ReleaseLease(ctx context.Context, machineID, nonce string) error {
	endpoint := fmt.Sprintf("/%s/lease", machineID)

	headers := make(map[string][]string)

	if nonce != "" {
		headers[NonceHeader] = []string{nonce}
	}

	return f.sendRequestMachines(ctx, machineReleaseLease, http.MethodDelete, endpoint, nil, nil, headers)
}

func (f *Client) Exec(ctx context.Context, machineID string, in *api.MachineExecRequest) (*api.MachineExecResponse, error) {
	endpoint := fmt.Sprintf("/%s/exec", machineID)

	out := new(api.MachineExecResponse)

	err := f.sendRequestMachines(ctx, machineExec, http.MethodPost, endpoint, in, out, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to exec on VM %s: %w", machineID, err)
	}
	return out, nil
}

func (f *Client) GetProcesses(ctx context.Context, machineID string) (api.MachinePsResponse, error) {
	endpoint := fmt.Sprintf("/%s/ps", machineID)

	var out api.MachinePsResponse

	err := f.sendRequestMachines(ctx, machinePs, http.MethodGet, endpoint, nil, &out, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get processes from VM %s: %w", machineID, err)
	}

	return out, nil
}

func (f *Client) Cordon(ctx context.Context, machineID string) (err error) {
	metrics.Started(ctx, "machine_cordon")
	sendUpdateMetrics := metrics.StartTiming(ctx, "machine_cordon/duration")
	defer func() {
		metrics.Status(ctx, "machine_cordon", err == nil)
		if err == nil {
			sendUpdateMetrics()
		}
	}()

	if err := f.sendRequestMachines(ctx, machineCordon, http.MethodPost, fmt.Sprintf("/%s/cordon", machineID), nil, nil, nil); err != nil {
		return fmt.Errorf("failed to cordon VM: %w", err)
	}

	return nil
}

func (f *Client) Uncordon(ctx context.Context, machineID string) (err error) {
	metrics.Started(ctx, "machine_uncordon")
	sendUpdateMetrics := metrics.StartTiming(ctx, "machine_uncordon/duration")
	defer func() {
		metrics.Status(ctx, "machine_uncordon", err == nil)
		if err == nil {
			sendUpdateMetrics()
		}
	}()

	if err := f.sendRequestMachines(ctx, machineUncordon, http.MethodPost, fmt.Sprintf("/%s/uncordon", machineID), nil, nil, nil); err != nil {
		return fmt.Errorf("failed to uncordon VM: %w", err)
	}

	return nil
}
