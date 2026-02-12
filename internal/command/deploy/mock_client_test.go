package deploy

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	fly "github.com/superfly/fly-go"
	"github.com/superfly/fly-go/flaps"
)

type mockWebClient struct {
}

func (f *mockWebClient) CanPerformBluegreenDeployment(ctx context.Context, appName string) (bool, error) {
	return true, nil
}

type mockFlapsClient struct {
	breakLaunch      bool
	breakWait        bool
	breakUncordon    bool
	breakSetMetadata bool
	breakList        bool
	breakDestroy     bool
	breakLease       bool

	// mu to protect the members below.
	mu            sync.Mutex
	machines      []*fly.Machine
	leases        map[string]struct{}
	nextMachineID int
}

func (m *mockFlapsClient) AcquireLease(ctx context.Context, appName, machineID string, ttl *int) (*fly.MachineLease, error) {
	nonce := fmt.Sprintf("%x-lease", machineID)
	return m.RefreshLease(ctx, appName, machineID, ttl, nonce)
}

func (m *mockFlapsClient) AssignIP(ctx context.Context, appName string, req flaps.AssignIPRequest) (res *flaps.IPAssignment, err error) {
	return nil, fmt.Errorf("failed to assign IP to %s", appName)
}

func (m *mockFlapsClient) CheckCertificate(ctx context.Context, appName, hostname string) (*fly.CertificateDetailResponse, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *mockFlapsClient) Cordon(ctx context.Context, appName, machineID string, nonce string) (err error) {
	return fmt.Errorf("failed to cordon %s", machineID)
}

func (m *mockFlapsClient) CreateACMECertificate(ctx context.Context, appName string, req fly.CreateCertificateRequest) (*fly.CertificateDetailResponse, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *mockFlapsClient) CreateApp(ctx context.Context, req flaps.CreateAppRequest) (*flaps.App, error) {
	return nil, fmt.Errorf("failed to create app")
}

func (m *mockFlapsClient) CreateCustomCertificate(ctx context.Context, appName string, req fly.ImportCertificateRequest) (*fly.CertificateDetailResponse, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *mockFlapsClient) CreateVolume(ctx context.Context, appName string, req fly.CreateVolumeRequest) (*fly.Volume, error) {
	return nil, fmt.Errorf("failed to create volume %s", req.Name)
}

func (m *mockFlapsClient) CreateVolumeSnapshot(ctx context.Context, appName, volumeId string) error {
	return fmt.Errorf("failed to create volume snapshot %s", volumeId)
}

func (m *mockFlapsClient) DeleteACMECertificate(ctx context.Context, appName, hostname string) error {
	return fmt.Errorf("not implemented")
}

func (m *mockFlapsClient) DeleteApp(ctx context.Context, name string) error {
	return fmt.Errorf("failed to delete app %s", name)
}

func (m *mockFlapsClient) DeleteCertificate(ctx context.Context, appName, hostname string) error {
	return fmt.Errorf("not implemented")
}

func (m *mockFlapsClient) DeleteCustomCertificate(ctx context.Context, appName, hostname string) error {
	return fmt.Errorf("not implemented")
}

func (m *mockFlapsClient) DeleteMetadata(ctx context.Context, appName, machineID, key string) error {
	return fmt.Errorf("failed to delete metadata %s", key)
}

func (m *mockFlapsClient) DeleteAppSecret(ctx context.Context, appName, name string) (*fly.DeleteAppSecretResp, error) {
	return nil, fmt.Errorf("failed to delete app secret %s", name)
}

func (m *mockFlapsClient) DeleteIPAssignment(ctx context.Context, appName, ip string) (err error) {
	return fmt.Errorf("failed to delete IP assignment %s from %s", ip, appName)
}

func (m *mockFlapsClient) DeleteSecretKey(ctx context.Context, appName, name string) error {
	return fmt.Errorf("failed to delete secret key %s", name)
}

func (m *mockFlapsClient) DeleteVolume(ctx context.Context, appName, volumeId string) (*fly.Volume, error) {
	return nil, fmt.Errorf("failed to delete volume %s", volumeId)
}

func (m *mockFlapsClient) Destroy(ctx context.Context, appName string, input fly.RemoveMachineInput, nonce string) (err error) {
	if m.breakDestroy {
		return fmt.Errorf("failed to destroy %s", input.ID)
	}
	return nil
}

func (m *mockFlapsClient) Exec(ctx context.Context, appName, machineID string, in *fly.MachineExecRequest) (*fly.MachineExecResponse, error) {
	return nil, fmt.Errorf("failed to exec %s", machineID)
}

func (m *mockFlapsClient) ExtendVolume(ctx context.Context, appName, volumeId string, size_gb int) (*fly.Volume, bool, error) {
	return nil, false, fmt.Errorf("failed to extend volume %s", volumeId)
}

func (m *mockFlapsClient) FindLease(ctx context.Context, appName, machineID string) (*fly.MachineLease, error) {
	return nil, fmt.Errorf("failed to find lease for %s", machineID)
}

func (m *mockFlapsClient) GenerateSecretKey(ctx context.Context, appName, name, typ string) (*fly.SetSecretKeyResp, error) {

	return nil, fmt.Errorf("failed to generate app secret %s", name)
}

func (m *mockFlapsClient) Get(ctx context.Context, appName, machineID string) (*fly.Machine, error) {
	return nil, fmt.Errorf("failed to get %s", machineID)
}

func (m *mockFlapsClient) GetApp(ctx context.Context, name string) (*flaps.App, error) {
	return nil, fmt.Errorf("failed to get app %s", name)
}

func (m *mockFlapsClient) GetAllVolumes(ctx context.Context, appName string) ([]fly.Volume, error) {
	return nil, fmt.Errorf("failed to get all volumes")
}

func (m *mockFlapsClient) GetCertificate(ctx context.Context, appName, hostname string) (*fly.CertificateDetailResponse, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *mockFlapsClient) GetIPAssignments(ctx context.Context, appName string) (res *flaps.ListIPAssignmentsResponse, err error) {
	return nil, fmt.Errorf("failed to get IP assignments for %s", appName)
}

func (m *mockFlapsClient) GetMany(ctx context.Context, appName string, machineIDs []string) ([]*fly.Machine, error) {
	return nil, fmt.Errorf("failed to get machines")
}

func (m *mockFlapsClient) GetMetadata(ctx context.Context, appName, machineID string) (map[string]string, error) {
	return nil, fmt.Errorf("failed to get metadata for %s", machineID)
}

func (m *mockFlapsClient) GetPlacements(ctx context.Context, req *flaps.GetPlacementsRequest) ([]flaps.RegionPlacement, error) {
	return nil, fmt.Errorf("failed to get placements")
}

func (m *mockFlapsClient) GetProcesses(ctx context.Context, appName, machineID string) (fly.MachinePsResponse, error) {
	return nil, fmt.Errorf("failed to get processes for %s", machineID)
}

func (m *mockFlapsClient) GetRegions(ctx context.Context) (*flaps.RegionData, error) {
	return nil, fmt.Errorf("failed to get regions")
}

func (m *mockFlapsClient) GetVolume(ctx context.Context, appName, volumeId string) (*fly.Volume, error) {
	return nil, fmt.Errorf("failed to get volume %s", volumeId)
}

func (m *mockFlapsClient) GetVolumeSnapshots(ctx context.Context, appName, volumeId string) ([]fly.VolumeSnapshot, error) {
	return nil, fmt.Errorf("failed to get volume snapshots for %s", volumeId)
}

func (m *mockFlapsClient) GetVolumes(ctx context.Context, appName string) ([]fly.Volume, error) {
	return nil, fmt.Errorf("failed to get volumes")
}

func (m *mockFlapsClient) Kill(ctx context.Context, appName, machineID string) (err error) {
	return fmt.Errorf("failed to kill %s", machineID)
}

func (m *mockFlapsClient) Launch(ctx context.Context, appName string, builder fly.LaunchMachineInput) (*fly.Machine, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.breakLaunch {
		return nil, fmt.Errorf("failed to launch %s", builder.ID)
	}
	m.nextMachineID += 1
	return &fly.Machine{
		ID:         fmt.Sprintf("%x", m.nextMachineID),
		LeaseNonce: fmt.Sprintf("%x-launch-lease", m.nextMachineID),
	}, nil
}

func (m *mockFlapsClient) List(ctx context.Context, appName, state string) ([]*fly.Machine, error) {
	if m.breakList {
		return nil, fmt.Errorf("failed to list machines")
	}
	return m.machines, nil
}

func (m *mockFlapsClient) ListActive(ctx context.Context, appName string) ([]*fly.Machine, error) {
	return nil, fmt.Errorf("failed to list active machines")
}

func (m *mockFlapsClient) ListApps(ctx context.Context, req flaps.ListAppsRequest) ([]flaps.App, error) {
	return nil, fmt.Errorf("failed to list apps")
}

func (m *mockFlapsClient) ListCertificates(ctx context.Context, appName string, opts *flaps.ListCertificatesOpts) (*fly.ListCertificatesResponse, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *mockFlapsClient) ListFlyAppsMachines(ctx context.Context, appName string) ([]*fly.Machine, *fly.Machine, error) {
	return nil, nil, fmt.Errorf("failed to list fly apps machines")
}

func (m *mockFlapsClient) ListAppSecrets(ctx context.Context, appName string, version *uint64, showSecrets bool) ([]fly.AppSecret, error) {
	return nil, fmt.Errorf("failed to list app secrets")
}

func (m *mockFlapsClient) ListSecretKeys(ctx context.Context, appName string, version *uint64) ([]fly.SecretKey, error) {
	return nil, fmt.Errorf("failed to list secret keys")
}

func (m *mockFlapsClient) NewRequest(ctx context.Context, method, path string, in interface{}, headers map[string][]string) (*http.Request, error) {
	return nil, fmt.Errorf("failed to create request")
}

func (m *mockFlapsClient) RefreshLease(ctx context.Context, appName, machineID string, ttl *int, nonce string) (*fly.MachineLease, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.breakLease {
		return nil, fmt.Errorf("failed to acquire lease for %s", machineID)
	}

	if m.leases == nil {
		m.leases = make(map[string]struct{})
	}
	m.leases[machineID] = struct{}{}

	return &fly.MachineLease{
		Status: "success",
		Data:   &fly.MachineLeaseData{Nonce: nonce},
	}, nil
}

func (m *mockFlapsClient) ReleaseLease(ctx context.Context, appName, machineID, nonce string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	_, exists := m.leases[machineID]
	if !exists {
		return fmt.Errorf("failed to release lease for %s", machineID)
	}
	delete(m.leases, machineID)

	return nil
}

func (m *mockFlapsClient) Restart(ctx context.Context, appName string, in fly.RestartMachineInput, nonce string) (err error) {
	return fmt.Errorf("failed to restart %s", in.ID)
}

func (m *mockFlapsClient) SetMetadata(ctx context.Context, appName, machineID, key, value string) error {
	if m.breakSetMetadata {
		return fmt.Errorf("failed to set metadata for %s", machineID)
	}
	return nil
}

func (m *mockFlapsClient) SetAppSecret(ctx context.Context, appName, name, value string) (*fly.SetAppSecretResp, error) {
	return nil, fmt.Errorf("failed to set app secret %s", name)
}

func (m *mockFlapsClient) SetSecretKey(ctx context.Context, appName, name, typ string, value []byte) (*fly.SetSecretKeyResp, error) {
	return nil, fmt.Errorf("failed to set secret key %s", name)
}

func (m *mockFlapsClient) Start(ctx context.Context, appName, machineID string, nonce string) (out *fly.MachineStartResponse, err error) {
	return nil, fmt.Errorf("failed to start %s", machineID)
}

func (m *mockFlapsClient) Stop(ctx context.Context, appName string, in fly.StopMachineInput, nonce string) (err error) {
	return fmt.Errorf("failed to stop %s", in.ID)
}

func (m *mockFlapsClient) Suspend(ctx context.Context, appName, machineID, nonce string) (err error) {
	return fmt.Errorf("failed to suspend %s", machineID)
}

func (m *mockFlapsClient) Uncordon(ctx context.Context, appName, machineID string, nonce string) (err error) {
	if m.breakUncordon {
		return fmt.Errorf("failed to uncordon %s", machineID)
	}
	return nil
}

func (m *mockFlapsClient) Update(ctx context.Context, appName string, builder fly.LaunchMachineInput, nonce string) (out *fly.Machine, err error) {
	return nil, fmt.Errorf("failed to update %s", builder.ID)
}

func (m *mockFlapsClient) UpdateAppSecrets(ctx context.Context, appName string, values map[string]*string) (*fly.UpdateAppSecretsResp, error) {
	return nil, fmt.Errorf("failed to update app secret %v", values)
}

func (m *mockFlapsClient) UpdateVolume(ctx context.Context, appName, volumeId string, req fly.UpdateVolumeRequest) (*fly.Volume, error) {
	return nil, fmt.Errorf("failed to update volume %s", volumeId)
}

func (m *mockFlapsClient) Wait(ctx context.Context, appName string, machine *fly.Machine, state string, timeout time.Duration) (err error) {
	if m.breakWait {
		return fmt.Errorf("failed to wait for %s", machine.ID)
	}
	return nil
}

func (m *mockFlapsClient) WaitForApp(ctx context.Context, name string) error {
	return fmt.Errorf("failed to wait for app %s", name)
}
