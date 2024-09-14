package mock

import (
	"context"
	"net/http"
	"time"

	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/internal/flapsutil"
)

var _ flapsutil.FlapsClient = (*FlapsClient)(nil)

type FlapsClient struct {
	AcquireLeaseFunc         func(ctx context.Context, machineID string, ttl *int) (*fly.MachineLease, error)
	CordonFunc               func(ctx context.Context, machineID string, nonce string) (err error)
	CreateAppFunc            func(ctx context.Context, name string, org string) (err error)
	CreateSecretFunc         func(ctx context.Context, sLabel, sType string, in fly.CreateSecretRequest) (err error)
	CreateVolumeFunc         func(ctx context.Context, req fly.CreateVolumeRequest) (*fly.Volume, error)
	CreateVolumeSnapshotFunc func(ctx context.Context, volumeId string) error
	DeleteMetadataFunc       func(ctx context.Context, machineID, key string) error
	DeleteSecretFunc         func(ctx context.Context, label string) (err error)
	DeleteVolumeFunc         func(ctx context.Context, volumeId string) (*fly.Volume, error)
	DestroyFunc              func(ctx context.Context, input fly.RemoveMachineInput, nonce string) (err error)
	ExecFunc                 func(ctx context.Context, machineID string, in *fly.MachineExecRequest) (*fly.MachineExecResponse, error)
	ExtendVolumeFunc         func(ctx context.Context, volumeId string, size_gb int) (*fly.Volume, bool, error)
	FindLeaseFunc            func(ctx context.Context, machineID string) (*fly.MachineLease, error)
	GenerateSecretFunc       func(ctx context.Context, sLabel, sType string) (err error)
	GetFunc                  func(ctx context.Context, machineID string) (*fly.Machine, error)
	GetAllVolumesFunc        func(ctx context.Context) ([]fly.Volume, error)
	GetManyFunc              func(ctx context.Context, machineIDs []string) ([]*fly.Machine, error)
	GetMetadataFunc          func(ctx context.Context, machineID string) (map[string]string, error)
	GetProcessesFunc         func(ctx context.Context, machineID string) (fly.MachinePsResponse, error)
	GetVolumeFunc            func(ctx context.Context, volumeId string) (*fly.Volume, error)
	GetVolumeSnapshotsFunc   func(ctx context.Context, volumeId string) ([]fly.VolumeSnapshot, error)
	GetVolumesFunc           func(ctx context.Context) ([]fly.Volume, error)
	KillFunc                 func(ctx context.Context, machineID string) (err error)
	LaunchFunc               func(ctx context.Context, builder fly.LaunchMachineInput) (out *fly.Machine, err error)
	ListFunc                 func(ctx context.Context, state string) ([]*fly.Machine, error)
	ListActiveFunc           func(ctx context.Context) ([]*fly.Machine, error)
	ListFlyAppsMachinesFunc  func(ctx context.Context) ([]*fly.Machine, *fly.Machine, error)
	ListSecretsFunc          func(ctx context.Context) (out []fly.ListSecret, err error)
	NewRequestFunc           func(ctx context.Context, method, path string, in interface{}, headers map[string][]string) (*http.Request, error)
	RefreshLeaseFunc         func(ctx context.Context, machineID string, ttl *int, nonce string) (*fly.MachineLease, error)
	ReleaseLeaseFunc         func(ctx context.Context, machineID, nonce string) error
	RestartFunc              func(ctx context.Context, in fly.RestartMachineInput, nonce string) (err error)
	SetMetadataFunc          func(ctx context.Context, machineID, key, value string) error
	StartFunc                func(ctx context.Context, machineID string, nonce string) (out *fly.MachineStartResponse, err error)
	StopFunc                 func(ctx context.Context, in fly.StopMachineInput, nonce string) (err error)
	SuspendFunc              func(ctx context.Context, machineID, nonce string) (err error)
	UncordonFunc             func(ctx context.Context, machineID string, nonce string) (err error)
	UpdateFunc               func(ctx context.Context, builder fly.LaunchMachineInput, nonce string) (out *fly.Machine, err error)
	UpdateVolumeFunc         func(ctx context.Context, volumeId string, req fly.UpdateVolumeRequest) (*fly.Volume, error)
	WaitFunc                 func(ctx context.Context, machine *fly.Machine, state string, timeout time.Duration) (err error)
	WaitForAppFunc           func(ctx context.Context, name string) error
}

func (m *FlapsClient) AcquireLease(ctx context.Context, machineID string, ttl *int) (*fly.MachineLease, error) {
	return m.AcquireLeaseFunc(ctx, machineID, ttl)
}

func (m *FlapsClient) Cordon(ctx context.Context, machineID string, nonce string) (err error) {
	return m.CordonFunc(ctx, machineID, nonce)
}

func (m *FlapsClient) CreateApp(ctx context.Context, name string, org string) (err error) {
	return m.CreateAppFunc(ctx, name, org)
}

func (m *FlapsClient) CreateSecret(ctx context.Context, sLabel, sType string, in fly.CreateSecretRequest) (err error) {
	return m.CreateSecretFunc(ctx, sLabel, sType, in)
}

func (m *FlapsClient) CreateVolume(ctx context.Context, req fly.CreateVolumeRequest) (*fly.Volume, error) {
	return m.CreateVolumeFunc(ctx, req)
}

func (m *FlapsClient) CreateVolumeSnapshot(ctx context.Context, volumeId string) error {
	return m.CreateVolumeSnapshotFunc(ctx, volumeId)
}

func (m *FlapsClient) DeleteMetadata(ctx context.Context, machineID, key string) error {
	return m.DeleteMetadataFunc(ctx, machineID, key)
}

func (m *FlapsClient) DeleteSecret(ctx context.Context, label string) (err error) {
	return m.DeleteSecretFunc(ctx, label)
}

func (m *FlapsClient) DeleteVolume(ctx context.Context, volumeId string) (*fly.Volume, error) {
	return m.DeleteVolumeFunc(ctx, volumeId)
}

func (m *FlapsClient) Destroy(ctx context.Context, input fly.RemoveMachineInput, nonce string) (err error) {
	return m.DestroyFunc(ctx, input, nonce)
}

func (m *FlapsClient) Exec(ctx context.Context, machineID string, in *fly.MachineExecRequest) (*fly.MachineExecResponse, error) {
	return m.ExecFunc(ctx, machineID, in)
}

func (m *FlapsClient) ExtendVolume(ctx context.Context, volumeId string, size_gb int) (*fly.Volume, bool, error) {
	return m.ExtendVolumeFunc(ctx, volumeId, size_gb)
}

func (m *FlapsClient) FindLease(ctx context.Context, machineID string) (*fly.MachineLease, error) {
	return m.FindLeaseFunc(ctx, machineID)
}

func (m *FlapsClient) GenerateSecret(ctx context.Context, sLabel, sType string) (err error) {
	return m.GenerateSecretFunc(ctx, sLabel, sType)
}

func (m *FlapsClient) Get(ctx context.Context, machineID string) (*fly.Machine, error) {
	return m.GetFunc(ctx, machineID)
}

func (m *FlapsClient) GetAllVolumes(ctx context.Context) ([]fly.Volume, error) {
	return m.GetAllVolumesFunc(ctx)
}

func (m *FlapsClient) GetMany(ctx context.Context, machineIDs []string) ([]*fly.Machine, error) {
	return m.GetManyFunc(ctx, machineIDs)
}

func (m *FlapsClient) GetMetadata(ctx context.Context, machineID string) (map[string]string, error) {
	return m.GetMetadataFunc(ctx, machineID)
}

func (m *FlapsClient) GetProcesses(ctx context.Context, machineID string) (fly.MachinePsResponse, error) {
	return m.GetProcessesFunc(ctx, machineID)
}

func (m *FlapsClient) GetVolume(ctx context.Context, volumeId string) (*fly.Volume, error) {
	return m.GetVolumeFunc(ctx, volumeId)
}

func (m *FlapsClient) GetVolumeSnapshots(ctx context.Context, volumeId string) ([]fly.VolumeSnapshot, error) {
	return m.GetVolumeSnapshotsFunc(ctx, volumeId)
}

func (m *FlapsClient) GetVolumes(ctx context.Context) ([]fly.Volume, error) {
	return m.GetVolumesFunc(ctx)
}

func (m *FlapsClient) Kill(ctx context.Context, machineID string) (err error) {
	return m.KillFunc(ctx, machineID)
}

func (m *FlapsClient) Launch(ctx context.Context, builder fly.LaunchMachineInput) (out *fly.Machine, err error) {
	return m.LaunchFunc(ctx, builder)
}

func (m *FlapsClient) List(ctx context.Context, state string) ([]*fly.Machine, error) {
	return m.ListFunc(ctx, state)
}

func (m *FlapsClient) ListActive(ctx context.Context) ([]*fly.Machine, error) {
	return m.ListActiveFunc(ctx)
}

func (m *FlapsClient) ListFlyAppsMachines(ctx context.Context) ([]*fly.Machine, *fly.Machine, error) {
	return m.ListFlyAppsMachinesFunc(ctx)
}

func (m *FlapsClient) ListSecrets(ctx context.Context) (out []fly.ListSecret, err error) {
	return m.ListSecretsFunc(ctx)
}

func (m *FlapsClient) NewRequest(ctx context.Context, method, path string, in interface{}, headers map[string][]string) (*http.Request, error) {
	return m.NewRequestFunc(ctx, method, path, in, headers)
}

func (m *FlapsClient) RefreshLease(ctx context.Context, machineID string, ttl *int, nonce string) (*fly.MachineLease, error) {
	return m.RefreshLeaseFunc(ctx, machineID, ttl, nonce)
}

func (m *FlapsClient) ReleaseLease(ctx context.Context, machineID, nonce string) error {
	return m.ReleaseLeaseFunc(ctx, machineID, nonce)
}

func (m *FlapsClient) Restart(ctx context.Context, in fly.RestartMachineInput, nonce string) (err error) {
	return m.RestartFunc(ctx, in, nonce)
}

func (m *FlapsClient) SetMetadata(ctx context.Context, machineID, key, value string) error {
	return m.SetMetadataFunc(ctx, machineID, key, value)
}

func (m *FlapsClient) Start(ctx context.Context, machineID string, nonce string) (out *fly.MachineStartResponse, err error) {
	return m.StartFunc(ctx, machineID, nonce)
}

func (m *FlapsClient) Stop(ctx context.Context, in fly.StopMachineInput, nonce string) (err error) {
	return m.StopFunc(ctx, in, nonce)
}

func (m *FlapsClient) Suspend(ctx context.Context, machineID, nonce string) (err error) {
	return m.SuspendFunc(ctx, machineID, nonce)
}

func (m *FlapsClient) Uncordon(ctx context.Context, machineID string, nonce string) (err error) {
	return m.UncordonFunc(ctx, machineID, nonce)
}

func (m *FlapsClient) Update(ctx context.Context, builder fly.LaunchMachineInput, nonce string) (out *fly.Machine, err error) {
	return m.UpdateFunc(ctx, builder, nonce)
}

func (m *FlapsClient) UpdateVolume(ctx context.Context, volumeId string, req fly.UpdateVolumeRequest) (*fly.Volume, error) {
	return m.UpdateVolumeFunc(ctx, volumeId, req)
}

func (m *FlapsClient) Wait(ctx context.Context, machine *fly.Machine, state string, timeout time.Duration) (err error) {
	return m.WaitFunc(ctx, machine, state, timeout)
}

func (m *FlapsClient) WaitForApp(ctx context.Context, name string) error {
	return m.WaitForAppFunc(ctx, name)
}
