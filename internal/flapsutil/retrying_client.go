package flapsutil

import (
	"context"
	"net/http"
	"time"

	"github.com/superfly/fly-go"
)

type retryClient struct {
	inner FlapsClient
}

func wrapWithRetry(inner FlapsClient) FlapsClient {
	return &retryClient{inner}
}

func (r *retryClient) AcquireLease(ctx context.Context, machineID string, ttl *int) (*fly.MachineLease, error) {
	return RetryRet(func() (*fly.MachineLease, error) {
		return r.inner.AcquireLease(ctx, machineID, ttl)
	})
}
func (r *retryClient) Cordon(ctx context.Context, machineID string, nonce string) (err error) {
	return Retry(func() error {
		return r.inner.Cordon(ctx, machineID, nonce)
	})
}
func (r *retryClient) CreateApp(ctx context.Context, name string, org string) (err error) {
	return Retry(func() error {
		return r.inner.CreateApp(ctx, name, org)
	})
}
func (r *retryClient) CreateVolume(ctx context.Context, req fly.CreateVolumeRequest) (*fly.Volume, error) {
	return RetryRet(func() (*fly.Volume, error) {
		return r.inner.CreateVolume(ctx, req)
	})
}
func (r *retryClient) CreateVolumeSnapshot(ctx context.Context, volumeId string) error {
	return Retry(func() error {
		return r.inner.CreateVolumeSnapshot(ctx, volumeId)
	})
}
func (r *retryClient) DeleteMetadata(ctx context.Context, machineID, key string) error {
	return Retry(func() error {
		return r.inner.DeleteMetadata(ctx, machineID, key)
	})
}
func (r *retryClient) DeleteVolume(ctx context.Context, volumeId string) (*fly.Volume, error) {
	return RetryRet(func() (*fly.Volume, error) {
		return r.inner.DeleteVolume(ctx, volumeId)
	})
}
func (r *retryClient) Destroy(ctx context.Context, input fly.RemoveMachineInput, nonce string) (err error) {
	return Retry(func() error {
		return r.inner.Destroy(ctx, input, nonce)
	})
}
func (r *retryClient) Exec(ctx context.Context, machineID string, in *fly.MachineExecRequest) (*fly.MachineExecResponse, error) {
	return RetryRet(func() (*fly.MachineExecResponse, error) {
		return r.inner.Exec(ctx, machineID, in)
	})
}
func (r *retryClient) ExtendVolume(ctx context.Context, volumeId string, size_gb int) (*fly.Volume, bool, error) {
	// RetryRet only works for functions that return one value and an error
	var (
		retVol         *fly.Volume
		retNeedRestart bool
	)
	err := Retry(func() error {
		var err error
		retVol, retNeedRestart, err = r.inner.ExtendVolume(ctx, volumeId, size_gb)
		return err
	})
	return retVol, retNeedRestart, err
}
func (r *retryClient) FindLease(ctx context.Context, machineID string) (*fly.MachineLease, error) {
	return RetryRet(func() (*fly.MachineLease, error) {
		return r.inner.FindLease(ctx, machineID)
	})
}
func (r *retryClient) Get(ctx context.Context, machineID string) (*fly.Machine, error) {
	return RetryRet(func() (*fly.Machine, error) {
		return r.inner.Get(ctx, machineID)
	})
}
func (r *retryClient) GetAllVolumes(ctx context.Context) ([]fly.Volume, error) {
	return RetryRet(func() ([]fly.Volume, error) {
		return r.inner.GetAllVolumes(ctx)
	})
}
func (r *retryClient) GetMany(ctx context.Context, machineIDs []string) ([]*fly.Machine, error) {
	return RetryRet(func() ([]*fly.Machine, error) {
		return r.inner.GetMany(ctx, machineIDs)
	})
}
func (r *retryClient) GetMetadata(ctx context.Context, machineID string) (map[string]string, error) {
	return RetryRet(func() (map[string]string, error) {
		return r.inner.GetMetadata(ctx, machineID)
	})
}
func (r *retryClient) GetProcesses(ctx context.Context, machineID string) (fly.MachinePsResponse, error) {
	return RetryRet(func() (fly.MachinePsResponse, error) {
		return r.inner.GetProcesses(ctx, machineID)
	})
}
func (r *retryClient) GetVolume(ctx context.Context, volumeId string) (*fly.Volume, error) {
	return RetryRet(func() (*fly.Volume, error) {
		return r.inner.GetVolume(ctx, volumeId)
	})
}
func (r *retryClient) GetVolumeSnapshots(ctx context.Context, volumeId string) ([]fly.VolumeSnapshot, error) {
	return RetryRet(func() ([]fly.VolumeSnapshot, error) {
		return r.inner.GetVolumeSnapshots(ctx, volumeId)
	})
}
func (r *retryClient) GetVolumes(ctx context.Context) ([]fly.Volume, error) {
	return RetryRet(func() ([]fly.Volume, error) {
		return r.inner.GetVolumes(ctx)
	})
}
func (r *retryClient) Kill(ctx context.Context, machineID string) (err error) {
	return Retry(func() error {
		return r.inner.Kill(ctx, machineID)
	})
}
func (r *retryClient) Launch(ctx context.Context, builder fly.LaunchMachineInput) (out *fly.Machine, err error) {
	return RetryRet(func() (*fly.Machine, error) {
		return r.inner.Launch(ctx, builder)
	})
}
func (r *retryClient) List(ctx context.Context, state string) ([]*fly.Machine, error) {
	return RetryRet(func() ([]*fly.Machine, error) {
		return r.inner.List(ctx, state)
	})
}
func (r *retryClient) ListActive(ctx context.Context) ([]*fly.Machine, error) {
	return RetryRet(func() ([]*fly.Machine, error) {
		return r.inner.ListActive(ctx)
	})
}
func (r *retryClient) ListFlyAppsMachines(ctx context.Context) ([]*fly.Machine, *fly.Machine, error) {
	// RetryRet only works for functions that return one value and an error
	var (
		retMachines      []*fly.Machine
		retRelCmdMachine *fly.Machine
	)
	err := Retry(func() error {
		var err error
		retMachines, retRelCmdMachine, err = r.inner.ListFlyAppsMachines(ctx)
		return err
	})
	return retMachines, retRelCmdMachine, err
}
func (r *retryClient) NewRequest(ctx context.Context, method, path string, in interface{}, headers map[string][]string) (*http.Request, error) {
	return RetryRet(func() (*http.Request, error) {
		return r.inner.NewRequest(ctx, method, path, in, headers)
	})
}
func (r *retryClient) RefreshLease(ctx context.Context, machineID string, ttl *int, nonce string) (*fly.MachineLease, error) {
	return RetryRet(func() (*fly.MachineLease, error) {
		return r.inner.RefreshLease(ctx, machineID, ttl, nonce)
	})
}
func (r *retryClient) ReleaseLease(ctx context.Context, machineID, nonce string) error {
	return Retry(func() error {
		return r.inner.ReleaseLease(ctx, machineID, nonce)
	})
}
func (r *retryClient) Restart(ctx context.Context, in fly.RestartMachineInput, nonce string) (err error) {
	return Retry(func() error {
		return r.inner.Restart(ctx, in, nonce)
	})
}
func (r *retryClient) SetMetadata(ctx context.Context, machineID, key, value string) error {
	return Retry(func() error {
		return r.inner.SetMetadata(ctx, machineID, key, value)
	})
}
func (r *retryClient) Start(ctx context.Context, machineID string, nonce string) (out *fly.MachineStartResponse, err error) {
	return RetryRet(func() (*fly.MachineStartResponse, error) {
		return r.inner.Start(ctx, machineID, nonce)
	})
}
func (r *retryClient) Stop(ctx context.Context, in fly.StopMachineInput, nonce string) (err error) {
	return Retry(func() error {
		return r.inner.Stop(ctx, in, nonce)
	})
}
func (r *retryClient) Uncordon(ctx context.Context, machineID string, nonce string) (err error) {
	return Retry(func() error {
		return r.inner.Uncordon(ctx, machineID, nonce)
	})
}
func (r *retryClient) Update(ctx context.Context, builder fly.LaunchMachineInput, nonce string) (out *fly.Machine, err error) {
	return RetryRet(func() (*fly.Machine, error) {
		return r.inner.Update(ctx, builder, nonce)
	})
}
func (r *retryClient) UpdateVolume(ctx context.Context, volumeId string, req fly.UpdateVolumeRequest) (*fly.Volume, error) {
	return RetryRet(func() (*fly.Volume, error) {
		return r.inner.UpdateVolume(ctx, volumeId, req)
	})
}
func (r *retryClient) Wait(ctx context.Context, machine *fly.Machine, state string, timeout time.Duration) (err error) {
	return Retry(func() error {
		return r.inner.Wait(ctx, machine, state, timeout)
	})
}
func (r *retryClient) WaitForApp(ctx context.Context, name string) error {
	return Retry(func() error {
		return r.inner.WaitForApp(ctx, name)
	})
}
