package deploy

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	fly "github.com/superfly/fly-go"
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

func (m *mockFlapsClient) AcquireLease(ctx context.Context, machineID string, ttl *int) (*fly.MachineLease, error) {
	nonce := fmt.Sprintf("%x-lease", machineID)
	return m.RefreshLease(ctx, machineID, ttl, nonce)
}

func (m *mockFlapsClient) Cordon(ctx context.Context, machineID string, nonce string) (err error) {
	return fmt.Errorf("failed to cordon %s", machineID)
}

func (m *mockFlapsClient) CreateApp(ctx context.Context, name string, org string) (err error) {
	return fmt.Errorf("failed to create app %s", name)
}

func (m *mockFlapsClient) CreateSecret(ctx context.Context, sLabel, sType string, in fly.CreateSecretRequest) (err error) {
	return fmt.Errorf("failed to create secret %s", sLabel)
}

func (m *mockFlapsClient) CreateVolume(ctx context.Context, req fly.CreateVolumeRequest) (*fly.Volume, error) {
	return nil, fmt.Errorf("failed to create volume %s", req.Name)
}

func (m *mockFlapsClient) CreateVolumeSnapshot(ctx context.Context, volumeId string) error {
	return fmt.Errorf("failed to create volume snapshot %s", volumeId)
}

func (m *mockFlapsClient) DeleteMetadata(ctx context.Context, machineID, key string) error {
	return fmt.Errorf("failed to delete metadata %s", key)
}

func (m *mockFlapsClient) DeleteSecret(ctx context.Context, label string) (err error) {
	return fmt.Errorf("failed to delete secret %s", label)
}

func (m *mockFlapsClient) DeleteVolume(ctx context.Context, volumeId string) (*fly.Volume, error) {
	return nil, fmt.Errorf("failed to delete volume %s", volumeId)
}

func (m *mockFlapsClient) Destroy(ctx context.Context, input fly.RemoveMachineInput, nonce string) (err error) {
	if m.breakDestroy {
		return fmt.Errorf("failed to destroy %s", input.ID)
	}
	return nil
}

func (m *mockFlapsClient) Exec(ctx context.Context, machineID string, in *fly.MachineExecRequest) (*fly.MachineExecResponse, error) {
	return nil, fmt.Errorf("failed to exec %s", machineID)
}

func (m *mockFlapsClient) ExtendVolume(ctx context.Context, volumeId string, size_gb int) (*fly.Volume, bool, error) {
	return nil, false, fmt.Errorf("failed to extend volume %s", volumeId)
}

func (m *mockFlapsClient) FindLease(ctx context.Context, machineID string) (*fly.MachineLease, error) {
	return nil, fmt.Errorf("failed to find lease for %s", machineID)
}

func (m *mockFlapsClient) GenerateSecret(ctx context.Context, sLabel, sType string) (err error) {
	return fmt.Errorf("failed to generate secret %s", sLabel)
}

func (m *mockFlapsClient) Get(ctx context.Context, machineID string) (*fly.Machine, error) {
	return nil, fmt.Errorf("failed to get %s", machineID)
}

func (m *mockFlapsClient) GetAllVolumes(ctx context.Context) ([]fly.Volume, error) {
	return nil, fmt.Errorf("failed to get all volumes")
}

func (m *mockFlapsClient) GetMany(ctx context.Context, machineIDs []string) ([]*fly.Machine, error) {
	return nil, fmt.Errorf("failed to get machines")
}

func (m *mockFlapsClient) GetMetadata(ctx context.Context, machineID string) (map[string]string, error) {
	return nil, fmt.Errorf("failed to get metadata for %s", machineID)
}

func (m *mockFlapsClient) GetProcesses(ctx context.Context, machineID string) (fly.MachinePsResponse, error) {
	return nil, fmt.Errorf("failed to get processes for %s", machineID)
}

func (m *mockFlapsClient) GetVolume(ctx context.Context, volumeId string) (*fly.Volume, error) {
	return nil, fmt.Errorf("failed to get volume %s", volumeId)
}

func (m *mockFlapsClient) GetVolumeSnapshots(ctx context.Context, volumeId string) ([]fly.VolumeSnapshot, error) {
	return nil, fmt.Errorf("failed to get volume snapshots for %s", volumeId)
}

func (m *mockFlapsClient) GetVolumes(ctx context.Context) ([]fly.Volume, error) {
	return nil, fmt.Errorf("failed to get volumes")
}

func (m *mockFlapsClient) Kill(ctx context.Context, machineID string) (err error) {
	return fmt.Errorf("failed to kill %s", machineID)
}

func (m *mockFlapsClient) Launch(ctx context.Context, builder fly.LaunchMachineInput) (*fly.Machine, error) {
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

func (m *mockFlapsClient) List(ctx context.Context, state string) ([]*fly.Machine, error) {
	if m.breakList {
		return nil, fmt.Errorf("failed to list machines")
	}
	return m.machines, nil
}

func (m *mockFlapsClient) ListActive(ctx context.Context) ([]*fly.Machine, error) {
	return nil, fmt.Errorf("failed to list active machines")
}

func (m *mockFlapsClient) ListFlyAppsMachines(ctx context.Context) ([]*fly.Machine, *fly.Machine, error) {
	return nil, nil, fmt.Errorf("failed to list fly apps machines")
}

func (m *mockFlapsClient) ListSecrets(ctx context.Context) (out []fly.ListSecret, err error) {
	return nil, fmt.Errorf("failed to list secrets")
}

func (m *mockFlapsClient) NewRequest(ctx context.Context, method, path string, in interface{}, headers map[string][]string) (*http.Request, error) {
	return nil, fmt.Errorf("failed to create request")
}

func (m *mockFlapsClient) RefreshLease(ctx context.Context, machineID string, ttl *int, nonce string) (*fly.MachineLease, error) {
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

func (m *mockFlapsClient) ReleaseLease(ctx context.Context, machineID, nonce string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	_, exists := m.leases[machineID]
	if !exists {
		return fmt.Errorf("failed to release lease for %s", machineID)
	}
	delete(m.leases, machineID)

	return nil
}

func (m *mockFlapsClient) Restart(ctx context.Context, in fly.RestartMachineInput, nonce string) (err error) {
	return fmt.Errorf("failed to restart %s", in.ID)
}

func (m *mockFlapsClient) SetMetadata(ctx context.Context, machineID, key, value string) error {
	if m.breakSetMetadata {
		return fmt.Errorf("failed to set metadata for %s", machineID)
	}
	return nil
}

func (m *mockFlapsClient) Start(ctx context.Context, machineID string, nonce string) (out *fly.MachineStartResponse, err error) {
	return nil, fmt.Errorf("failed to start %s", machineID)
}

func (m *mockFlapsClient) Stop(ctx context.Context, in fly.StopMachineInput, nonce string) (err error) {
	return fmt.Errorf("failed to stop %s", in.ID)
}

func (m *mockFlapsClient) Suspend(ctx context.Context, machineID, nonce string) (err error) {
	return fmt.Errorf("failed to suspend %s", machineID)
}

func (m *mockFlapsClient) Uncordon(ctx context.Context, machineID string, nonce string) (err error) {
	if m.breakUncordon {
		return fmt.Errorf("failed to uncordon %s", machineID)
	}
	return nil
}

func (m *mockFlapsClient) Update(ctx context.Context, builder fly.LaunchMachineInput, nonce string) (out *fly.Machine, err error) {
	return nil, fmt.Errorf("failed to update %s", builder.ID)
}

func (m *mockFlapsClient) UpdateVolume(ctx context.Context, volumeId string, req fly.UpdateVolumeRequest) (*fly.Volume, error) {
	return nil, fmt.Errorf("failed to update volume %s", volumeId)
}

func (m *mockFlapsClient) Wait(ctx context.Context, machine *fly.Machine, state string, timeout time.Duration) (err error) {
	if m.breakWait {
		return fmt.Errorf("failed to wait for %s", machine.ID)
	}
	return nil
}

func (m *mockFlapsClient) WaitForApp(ctx context.Context, name string) error {
	return fmt.Errorf("failed to wait for app %s", name)
}
