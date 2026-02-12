package mock

import (
	"context"
	"net/http"
	"time"

	fly "github.com/superfly/fly-go"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/flapsutil"
)

var _ flapsutil.FlapsClient = (*FlapsClient)(nil)

type FlapsClient struct {
	AcquireLeaseFunc            func(ctx context.Context, appName, machineID string, ttl *int) (*fly.MachineLease, error)
	AssignIPFunc                func(ctx context.Context, appName string, req flaps.AssignIPRequest) (res *flaps.IPAssignment, err error)
	CheckCertificateFunc        func(ctx context.Context, appName, hostname string) (*fly.CertificateDetailResponse, error)
	CordonFunc                  func(ctx context.Context, appName, machineID string, nonce string) (err error)
	CreateAppFunc               func(ctx context.Context, req flaps.CreateAppRequest) (*flaps.App, error)
	CreateACMECertificateFunc   func(ctx context.Context, appName string, req fly.CreateCertificateRequest) (*fly.CertificateDetailResponse, error)
	CreateVolumeFunc            func(ctx context.Context, appName string, req fly.CreateVolumeRequest) (*fly.Volume, error)
	CreateVolumeSnapshotFunc    func(ctx context.Context, appName, volumeId string) error
	DeleteAppFunc               func(ctx context.Context, name string) error
	DeleteACMECertificateFunc   func(ctx context.Context, appName, hostname string) error
	DeleteCertificateFunc       func(ctx context.Context, appName, hostname string) error
	DeleteCustomCertificateFunc func(ctx context.Context, appName, hostname string) error
	DeleteMetadataFunc          func(ctx context.Context, appName, machineID, key string) error
	DeleteAppSecretFunc         func(ctx context.Context, appName, name string) (*fly.DeleteAppSecretResp, error)
	DeleteIPAssignmentFunc      func(ctx context.Context, appName, ip string) (err error)
	DeleteSecretKeyFunc         func(ctx context.Context, appName, name string) error
	DeleteVolumeFunc            func(ctx context.Context, appName, volumeId string) (*fly.Volume, error)
	DestroyFunc                 func(ctx context.Context, appName string, input fly.RemoveMachineInput, nonce string) (err error)
	ExecFunc                    func(ctx context.Context, appName, machineID string, in *fly.MachineExecRequest) (*fly.MachineExecResponse, error)
	ExtendVolumeFunc            func(ctx context.Context, appName, volumeId string, size_gb int) (*fly.Volume, bool, error)
	FindLeaseFunc               func(ctx context.Context, appName, machineID string) (*fly.MachineLease, error)
	GenerateSecretKeyFunc       func(ctx context.Context, appName, name string, typ string) (*fly.SetSecretKeyResp, error)
	GetFunc                     func(ctx context.Context, appName, machineID string) (*fly.Machine, error)
	GetAppFunc                  func(ctx context.Context, name string) (app *flaps.App, err error)
	GetAllVolumesFunc           func(ctx context.Context, appName string) ([]fly.Volume, error)
	GetCertificateFunc          func(ctx context.Context, appName, hostname string) (*fly.CertificateDetailResponse, error)
	GetIPAssignmentsFunc        func(ctx context.Context, appName string) (res *flaps.ListIPAssignmentsResponse, err error)
	GetManyFunc                 func(ctx context.Context, appName string, machineIDs []string) ([]*fly.Machine, error)
	GetMetadataFunc             func(ctx context.Context, appName, machineID string) (map[string]string, error)
	GetPlacementsFunc           func(ctx context.Context, req *flaps.GetPlacementsRequest) ([]flaps.RegionPlacement, error)
	GetProcessesFunc            func(ctx context.Context, appName, machineID string) (fly.MachinePsResponse, error)
	GetRegionsFunc              func(ctx context.Context) (*flaps.RegionData, error)
	GetVolumeFunc               func(ctx context.Context, appName, volumeId string) (*fly.Volume, error)
	GetVolumeSnapshotsFunc      func(ctx context.Context, appName, volumeId string) ([]fly.VolumeSnapshot, error)
	GetVolumesFunc              func(ctx context.Context, appName string) ([]fly.Volume, error)
	CreateCustomCertificateFunc func(ctx context.Context, appName string, req fly.ImportCertificateRequest) (*fly.CertificateDetailResponse, error)
	KillFunc                    func(ctx context.Context, appName, machineID string) (err error)
	LaunchFunc                  func(ctx context.Context, appName string, builder fly.LaunchMachineInput) (out *fly.Machine, err error)
	ListFunc                    func(ctx context.Context, appName, state string) ([]*fly.Machine, error)
	ListActiveFunc              func(ctx context.Context, appName string) ([]*fly.Machine, error)
	ListAppsFunc                func(ctx context.Context, req flaps.ListAppsRequest) ([]flaps.App, error)
	ListAppSecretsFunc          func(ctx context.Context, appName string, version *uint64, showSecrets bool) ([]fly.AppSecret, error)
	ListCertificatesFunc        func(ctx context.Context, appName string, opts *flaps.ListCertificatesOpts) (*fly.ListCertificatesResponse, error)
	ListFlyAppsMachinesFunc     func(ctx context.Context, appName string) ([]*fly.Machine, *fly.Machine, error)
	ListSecretKeysFunc          func(ctx context.Context, appName string, version *uint64) ([]fly.SecretKey, error)
	NewRequestFunc              func(ctx context.Context, method, path string, in interface{}, headers map[string][]string) (*http.Request, error)
	RefreshLeaseFunc            func(ctx context.Context, appName, machineID string, ttl *int, nonce string) (*fly.MachineLease, error)
	ReleaseLeaseFunc            func(ctx context.Context, appName, machineID, nonce string) error
	RestartFunc                 func(ctx context.Context, appName string, in fly.RestartMachineInput, nonce string) (err error)
	SetMetadataFunc             func(ctx context.Context, appName, machineID, key, value string) error
	SetAppSecretFunc            func(ctx context.Context, appName, name string, value string) (*fly.SetAppSecretResp, error)
	SetSecretKeyFunc            func(ctx context.Context, appName, name string, typ string, value []byte) (*fly.SetSecretKeyResp, error)
	StartFunc                   func(ctx context.Context, appName, machineID string, nonce string) (out *fly.MachineStartResponse, err error)
	StopFunc                    func(ctx context.Context, appName string, in fly.StopMachineInput, nonce string) (err error)
	SuspendFunc                 func(ctx context.Context, appName, machineID, nonce string) error
	UncordonFunc                func(ctx context.Context, appName, machineID string, nonce string) (err error)
	UpdateFunc                  func(ctx context.Context, appName string, builder fly.LaunchMachineInput, nonce string) (out *fly.Machine, err error)
	UpdateAppSecretsFunc        func(ctx context.Context, appName string, values map[string]*string) (*fly.UpdateAppSecretsResp, error)
	UpdateVolumeFunc            func(ctx context.Context, appName, volumeId string, req fly.UpdateVolumeRequest) (*fly.Volume, error)
	WaitFunc                    func(ctx context.Context, appName string, machine *fly.Machine, state string, timeout time.Duration) (err error)
	WaitForAppFunc              func(ctx context.Context, name string) error
}

func (m *FlapsClient) AcquireLease(ctx context.Context, appName, machineID string, ttl *int) (*fly.MachineLease, error) {
	return m.AcquireLeaseFunc(ctx, appName, machineID, ttl)
}

func (m *FlapsClient) AssignIP(ctx context.Context, appName string, req flaps.AssignIPRequest) (res *flaps.IPAssignment, err error) {
	return m.AssignIPFunc(ctx, appName, req)
}

func (m *FlapsClient) CheckCertificate(ctx context.Context, appName, hostname string) (*fly.CertificateDetailResponse, error) {
	return m.CheckCertificateFunc(ctx, appName, hostname)
}

func (m *FlapsClient) Cordon(ctx context.Context, appName, machineID string, nonce string) (err error) {
	return m.CordonFunc(ctx, appName, machineID, nonce)
}

func (m *FlapsClient) CreateApp(ctx context.Context, req flaps.CreateAppRequest) (*flaps.App, error) {
	return m.CreateAppFunc(ctx, req)
}

func (m *FlapsClient) CreateACMECertificate(ctx context.Context, appName string, req fly.CreateCertificateRequest) (*fly.CertificateDetailResponse, error) {
	return m.CreateACMECertificateFunc(ctx, appName, req)
}

func (m *FlapsClient) CreateVolume(ctx context.Context, appName string, req fly.CreateVolumeRequest) (*fly.Volume, error) {
	return m.CreateVolumeFunc(ctx, appName, req)
}

func (m *FlapsClient) CreateVolumeSnapshot(ctx context.Context, appName, volumeId string) error {
	return m.CreateVolumeSnapshotFunc(ctx, appName, volumeId)
}

func (m *FlapsClient) DeleteApp(ctx context.Context, name string) error {
	return m.DeleteAppFunc(ctx, name)
}

func (m *FlapsClient) DeleteACMECertificate(ctx context.Context, appName, hostname string) error {
	return m.DeleteACMECertificateFunc(ctx, appName, hostname)
}

func (m *FlapsClient) DeleteCertificate(ctx context.Context, appName, hostname string) error {
	return m.DeleteCertificateFunc(ctx, appName, hostname)
}

func (m *FlapsClient) DeleteCustomCertificate(ctx context.Context, appName, hostname string) error {
	return m.DeleteCustomCertificateFunc(ctx, appName, hostname)
}

func (m *FlapsClient) DeleteMetadata(ctx context.Context, appName, machineID, key string) error {
	return m.DeleteMetadataFunc(ctx, appName, machineID, key)
}

func (m *FlapsClient) DeleteAppSecret(ctx context.Context, appName, name string) (*fly.DeleteAppSecretResp, error) {
	return m.DeleteAppSecretFunc(ctx, appName, name)
}

func (m *FlapsClient) DeleteIPAssignment(ctx context.Context, appName, ip string) (err error) {
	return m.DeleteIPAssignmentFunc(ctx, appName, ip)
}

func (m *FlapsClient) DeleteSecretKey(ctx context.Context, appName, name string) error {
	return m.DeleteSecretKeyFunc(ctx, appName, name)
}

func (m *FlapsClient) DeleteVolume(ctx context.Context, appName, volumeId string) (*fly.Volume, error) {
	return m.DeleteVolumeFunc(ctx, appName, volumeId)
}

func (m *FlapsClient) Destroy(ctx context.Context, appName string, input fly.RemoveMachineInput, nonce string) (err error) {
	return m.DestroyFunc(ctx, appName, input, nonce)
}

func (m *FlapsClient) Exec(ctx context.Context, appName, machineID string, in *fly.MachineExecRequest) (*fly.MachineExecResponse, error) {
	return m.ExecFunc(ctx, appName, machineID, in)
}

func (m *FlapsClient) ExtendVolume(ctx context.Context, appName, volumeId string, size_gb int) (*fly.Volume, bool, error) {
	return m.ExtendVolumeFunc(ctx, appName, volumeId, size_gb)
}

func (m *FlapsClient) FindLease(ctx context.Context, appName, machineID string) (*fly.MachineLease, error) {
	return m.FindLeaseFunc(ctx, appName, machineID)
}

func (m *FlapsClient) GenerateSecretKey(ctx context.Context, appName, name string, typ string) (*fly.SetSecretKeyResp, error) {
	return m.GenerateSecretKeyFunc(ctx, appName, name, typ)
}

func (m *FlapsClient) Get(ctx context.Context, appName, machineID string) (*fly.Machine, error) {
	return m.GetFunc(ctx, appName, machineID)
}

func (m *FlapsClient) GetApp(ctx context.Context, name string) (app *flaps.App, err error) {
	return m.GetAppFunc(ctx, name)
}

func (m *FlapsClient) GetAllVolumes(ctx context.Context, appName string) ([]fly.Volume, error) {
	return m.GetAllVolumesFunc(ctx, appName)
}

func (m *FlapsClient) GetCertificate(ctx context.Context, appName, hostname string) (*fly.CertificateDetailResponse, error) {
	return m.GetCertificateFunc(ctx, appName, hostname)
}

func (m *FlapsClient) GetIPAssignments(ctx context.Context, appName string) (res *flaps.ListIPAssignmentsResponse, err error) {
	return m.GetIPAssignmentsFunc(ctx, appName)
}

func (m *FlapsClient) GetMany(ctx context.Context, appName string, machineIDs []string) ([]*fly.Machine, error) {
	return m.GetManyFunc(ctx, appName, machineIDs)
}

func (m *FlapsClient) GetMetadata(ctx context.Context, appName, machineID string) (map[string]string, error) {
	return m.GetMetadataFunc(ctx, appName, machineID)
}

func (m *FlapsClient) GetPlacements(ctx context.Context, req *flaps.GetPlacementsRequest) ([]flaps.RegionPlacement, error) {
	return m.GetPlacementsFunc(ctx, req)
}

func (m *FlapsClient) GetProcesses(ctx context.Context, appName, machineID string) (fly.MachinePsResponse, error) {
	return m.GetProcessesFunc(ctx, appName, machineID)
}

func (m *FlapsClient) GetRegions(ctx context.Context) (*flaps.RegionData, error) {
	return m.GetRegionsFunc(ctx)
}

func (m *FlapsClient) GetVolume(ctx context.Context, appName, volumeId string) (*fly.Volume, error) {
	return m.GetVolumeFunc(ctx, appName, volumeId)
}

func (m *FlapsClient) GetVolumeSnapshots(ctx context.Context, appName, volumeId string) ([]fly.VolumeSnapshot, error) {
	return m.GetVolumeSnapshotsFunc(ctx, appName, volumeId)
}

func (m *FlapsClient) GetVolumes(ctx context.Context, appName string) ([]fly.Volume, error) {
	return m.GetVolumesFunc(ctx, appName)
}

func (m *FlapsClient) CreateCustomCertificate(ctx context.Context, appName string, req fly.ImportCertificateRequest) (*fly.CertificateDetailResponse, error) {
	return m.CreateCustomCertificateFunc(ctx, appName, req)
}

func (m *FlapsClient) Kill(ctx context.Context, appName, machineID string) (err error) {
	return m.KillFunc(ctx, appName, machineID)
}

func (m *FlapsClient) Launch(ctx context.Context, appName string, builder fly.LaunchMachineInput) (out *fly.Machine, err error) {
	return m.LaunchFunc(ctx, appName, builder)
}

func (m *FlapsClient) List(ctx context.Context, appName, state string) ([]*fly.Machine, error) {
	return m.ListFunc(ctx, appName, state)
}

func (m *FlapsClient) ListActive(ctx context.Context, appName string) ([]*fly.Machine, error) {
	return m.ListActiveFunc(ctx, appName)
}

func (m *FlapsClient) ListApps(ctx context.Context, req flaps.ListAppsRequest) ([]flaps.App, error) {
	return m.ListAppsFunc(ctx, req)
}

func (m *FlapsClient) ListAppSecrets(ctx context.Context, appName string, version *uint64, showSecrets bool) ([]fly.AppSecret, error) {
	return m.ListAppSecretsFunc(ctx, appName, version, showSecrets)
}

func (m *FlapsClient) ListCertificates(ctx context.Context, appName string, opts *flaps.ListCertificatesOpts) (*fly.ListCertificatesResponse, error) {
	return m.ListCertificatesFunc(ctx, appName, opts)
}

func (m *FlapsClient) ListFlyAppsMachines(ctx context.Context, appName string) ([]*fly.Machine, *fly.Machine, error) {
	return m.ListFlyAppsMachinesFunc(ctx, appName)
}

func (m *FlapsClient) ListSecretKeys(ctx context.Context, appName string, version *uint64) ([]fly.SecretKey, error) {
	return m.ListSecretKeysFunc(ctx, appName, version)
}

func (m *FlapsClient) NewRequest(ctx context.Context, method, path string, in interface{}, headers map[string][]string) (*http.Request, error) {
	return m.NewRequestFunc(ctx, method, path, in, headers)
}

func (m *FlapsClient) RefreshLease(ctx context.Context, appName, machineID string, ttl *int, nonce string) (*fly.MachineLease, error) {
	return m.RefreshLeaseFunc(ctx, appName, machineID, ttl, nonce)
}

func (m *FlapsClient) ReleaseLease(ctx context.Context, appName, machineID, nonce string) error {
	return m.ReleaseLeaseFunc(ctx, appName, machineID, nonce)
}

func (m *FlapsClient) Restart(ctx context.Context, appName string, in fly.RestartMachineInput, nonce string) (err error) {
	return m.RestartFunc(ctx, appName, in, nonce)
}

func (m *FlapsClient) SetMetadata(ctx context.Context, appName, machineID, key, value string) error {
	return m.SetMetadataFunc(ctx, appName, machineID, key, value)
}

func (m *FlapsClient) SetAppSecret(ctx context.Context, appName, name string, value string) (*fly.SetAppSecretResp, error) {
	return m.SetAppSecretFunc(ctx, appName, name, value)
}

func (m *FlapsClient) SetSecretKey(ctx context.Context, appName, name string, typ string, value []byte) (*fly.SetSecretKeyResp, error) {
	return m.SetSecretKeyFunc(ctx, appName, name, typ, value)
}

func (m *FlapsClient) Start(ctx context.Context, appName, machineID string, nonce string) (out *fly.MachineStartResponse, err error) {
	return m.StartFunc(ctx, appName, machineID, nonce)
}

func (m *FlapsClient) Stop(ctx context.Context, appName string, in fly.StopMachineInput, nonce string) (err error) {
	return m.StopFunc(ctx, appName, in, nonce)
}

func (m *FlapsClient) Suspend(ctx context.Context, appName, machineID, nonce string) error {
	return m.SuspendFunc(ctx, appName, machineID, nonce)
}

func (m *FlapsClient) Uncordon(ctx context.Context, appName, machineID string, nonce string) (err error) {
	return m.UncordonFunc(ctx, appName, machineID, nonce)
}

func (m *FlapsClient) Update(ctx context.Context, appName string, builder fly.LaunchMachineInput, nonce string) (out *fly.Machine, err error) {
	return m.UpdateFunc(ctx, appName, builder, nonce)
}

func (m *FlapsClient) UpdateAppSecrets(ctx context.Context, appName string, values map[string]*string) (*fly.UpdateAppSecretsResp, error) {
	return m.UpdateAppSecretsFunc(ctx, appName, values)
}

func (m *FlapsClient) UpdateVolume(ctx context.Context, appName, volumeId string, req fly.UpdateVolumeRequest) (*fly.Volume, error) {
	return m.UpdateVolumeFunc(ctx, appName, volumeId, req)
}

func (m *FlapsClient) Wait(ctx context.Context, appName string, machine *fly.Machine, state string, timeout time.Duration) (err error) {
	return m.WaitFunc(ctx, appName, machine, state, timeout)
}

func (m *FlapsClient) WaitForApp(ctx context.Context, name string) error {
	return m.WaitForAppFunc(ctx, name)
}
