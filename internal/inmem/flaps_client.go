package inmem

import (
	"context"
	"fmt"
	"net/http"
	"time"

	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/internal/flapsutil"
)

var _ flapsutil.FlapsClient = (*FlapsClient)(nil)

type FlapsClient struct {
	server  *Server
	appName string
}

func NewFlapsClient(server *Server, appName string) *FlapsClient {
	return &FlapsClient{
		server:  server,
		appName: appName,
	}
}

func (m *FlapsClient) AcquireLease(ctx context.Context, machineID string, ttl *int) (*fly.MachineLease, error) {
	panic("TODO")
}

func (m *FlapsClient) Cordon(ctx context.Context, machineID string, nonce string) (err error) {
	panic("TODO")
}

func (m *FlapsClient) CreateApp(ctx context.Context, name string, org string) (err error) {
	panic("TODO")
}

func (m *FlapsClient) CreateSecret(ctx context.Context, sLabel, sType string, in fly.CreateSecretRequest) (err error) {
	panic("TODO")
}

func (m *FlapsClient) CreateVolume(ctx context.Context, req fly.CreateVolumeRequest) (*fly.Volume, error) {
	panic("TODO")
}

func (m *FlapsClient) CreateVolumeSnapshot(ctx context.Context, volumeId string) error {
	panic("TODO")
}

func (m *FlapsClient) DeleteMetadata(ctx context.Context, machineID, key string) error {
	panic("TODO")
}

func (m *FlapsClient) DeleteSecret(ctx context.Context, label string) (err error) {
	panic("TODO")
}

func (m *FlapsClient) DeleteVolume(ctx context.Context, volumeId string) (*fly.Volume, error) {
	panic("TODO")
}

func (m *FlapsClient) Destroy(ctx context.Context, input fly.RemoveMachineInput, nonce string) (err error) {
	panic("TODO")
}

func (m *FlapsClient) Exec(ctx context.Context, machineID string, in *fly.MachineExecRequest) (*fly.MachineExecResponse, error) {
	panic("TODO")
}

func (m *FlapsClient) ExtendVolume(ctx context.Context, volumeId string, size_gb int) (*fly.Volume, bool, error) {
	panic("TODO")
}

func (m *FlapsClient) FindLease(ctx context.Context, machineID string) (*fly.MachineLease, error) {
	panic("TODO")
}

func (m *FlapsClient) GenerateSecret(ctx context.Context, sLabel, sType string) (err error) {
	panic("TODO")
}

func (m *FlapsClient) Get(ctx context.Context, machineID string) (*fly.Machine, error) {
	return m.server.GetMachine(ctx, m.appName, machineID)
}

func (m *FlapsClient) GetAllVolumes(ctx context.Context) ([]fly.Volume, error) {
	panic("TODO")
}

func (m *FlapsClient) GetMany(ctx context.Context, machineIDs []string) ([]*fly.Machine, error) {
	panic("TODO")
}

func (m *FlapsClient) GetMetadata(ctx context.Context, machineID string) (map[string]string, error) {
	panic("TODO")
}

func (m *FlapsClient) GetProcesses(ctx context.Context, machineID string) (fly.MachinePsResponse, error) {
	return nil, nil // TODO
}

func (m *FlapsClient) GetVolume(ctx context.Context, volumeId string) (*fly.Volume, error) {
	panic("TODO")
}

func (m *FlapsClient) GetVolumeSnapshots(ctx context.Context, volumeId string) ([]fly.VolumeSnapshot, error) {
	panic("TODO")
}

func (m *FlapsClient) GetVolumes(ctx context.Context) ([]fly.Volume, error) {
	panic("TODO")
}

func (m *FlapsClient) Kill(ctx context.Context, machineID string) (err error) {
	panic("TODO")
}

func (m *FlapsClient) Launch(ctx context.Context, builder fly.LaunchMachineInput) (out *fly.Machine, err error) {
	return m.server.Launch(ctx, m.appName, builder.Name, builder.Region, builder.Config)
}

func (m *FlapsClient) List(ctx context.Context, state string) ([]*fly.Machine, error) {
	panic("TODO")
}

func (m *FlapsClient) ListActive(ctx context.Context) ([]*fly.Machine, error) {
	m.server.mu.Lock()
	defer m.server.mu.Unlock()

	var a []*fly.Machine
	for _, machine := range m.server.machines[m.appName] {
		if !machine.IsReleaseCommandMachine() && !machine.IsFlyAppsConsole() && machine.IsActive() {
			a = append(a, machine)
		}
	}
	return a, nil
}

func (m *FlapsClient) ListFlyAppsMachines(ctx context.Context) (machines []*fly.Machine, releaseCmdMachine *fly.Machine, err error) {
	m.server.mu.Lock()
	defer m.server.mu.Unlock()

	machines = make([]*fly.Machine, 0)
	for _, machine := range m.server.machines[m.appName] {
		if machine.IsFlyAppsPlatform() && machine.IsActive() && !machine.IsFlyAppsReleaseCommand() && !machine.IsFlyAppsConsole() {
			machines = append(machines, machine)
		} else if machine.IsFlyAppsReleaseCommand() {
			releaseCmdMachine = machine
		}
	}
	return machines, releaseCmdMachine, nil
}

func (m *FlapsClient) ListSecrets(ctx context.Context) (out []fly.ListSecret, err error) {
	panic("TODO")
}

func (m *FlapsClient) NewRequest(ctx context.Context, method, path string, in interface{}, headers map[string][]string) (*http.Request, error) {
	panic("TODO")
}

func (m *FlapsClient) RefreshLease(ctx context.Context, machineID string, ttl *int, nonce string) (*fly.MachineLease, error) {
	panic("TODO")
}

func (m *FlapsClient) ReleaseLease(ctx context.Context, machineID, nonce string) error {
	panic("TODO")
}

func (m *FlapsClient) Restart(ctx context.Context, in fly.RestartMachineInput, nonce string) (err error) {
	panic("TODO")
}

func (m *FlapsClient) SetMetadata(ctx context.Context, machineID, key, value string) error {
	panic("TODO")
}

func (m *FlapsClient) Start(ctx context.Context, machineID string, nonce string) (out *fly.MachineStartResponse, err error) {
	panic("TODO")
}

func (m *FlapsClient) Stop(ctx context.Context, in fly.StopMachineInput, nonce string) (err error) {
	panic("TODO")
}

func (m *FlapsClient) Suspend(ctx context.Context, machineID, nonce string) (err error) {
	panic("TODO")
}

func (m *FlapsClient) Uncordon(ctx context.Context, machineID string, nonce string) (err error) {
	panic("TODO")
}

func (m *FlapsClient) Update(ctx context.Context, builder fly.LaunchMachineInput, nonce string) (out *fly.Machine, err error) {
	panic("TODO")
}

func (m *FlapsClient) UpdateVolume(ctx context.Context, volumeId string, req fly.UpdateVolumeRequest) (*fly.Volume, error) {
	panic("TODO")
}

func (m *FlapsClient) Wait(ctx context.Context, machine *fly.Machine, state string, timeout time.Duration) (err error) {
	if state == "" {
		state = "started"
	}
	mach, err := m.server.GetMachine(ctx, m.appName, machine.ID)
	if err != nil {
		return err
	}
	if mach.State != state {
		return fmt.Errorf("machine did not reach state %q, current state is %q", state, mach.State)
	}
	return nil
}

func (m *FlapsClient) WaitForApp(ctx context.Context, name string) error {
	panic("TODO")
}
