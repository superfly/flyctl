package inmem

import (
	"context"
	"fmt"
	"net/http"
	"time"

	fly "github.com/superfly/fly-go"
	"github.com/superfly/fly-go/flaps"
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

func (m *FlapsClient) AcquireLease(ctx context.Context, appName, machineID string, ttl *int) (*fly.MachineLease, error) {
	panic("TODO")
}

func (m *FlapsClient) AssignIP(ctx context.Context, appName string, req flaps.AssignIPRequest) (res *flaps.IPAssignment, err error) {
	panic("TODO")
}

func (m *FlapsClient) CheckCertificate(ctx context.Context, appName, hostname string) (*fly.CertificateDetailResponse, error) {
	panic("TODO")
}

func (m *FlapsClient) Cordon(ctx context.Context, appName, machineID string, nonce string) (err error) {
	panic("TODO")
}

func (m *FlapsClient) CreateApp(ctx context.Context, req flaps.CreateAppRequest) (*flaps.App, error) {
	panic("TODO")
}

func (m *FlapsClient) CreateACMECertificate(ctx context.Context, appName string, req fly.CreateCertificateRequest) (*fly.CertificateDetailResponse, error) {
	panic("TODO")
}

func (m *FlapsClient) CreateVolume(ctx context.Context, appName string, req fly.CreateVolumeRequest) (*fly.Volume, error) {
	panic("TODO")
}

func (m *FlapsClient) CreateVolumeSnapshot(ctx context.Context, appName, volumeId string) error {
	panic("TODO")
}

func (m *FlapsClient) DeleteApp(ctx context.Context, name string) error {
	panic("TODO")
}

func (m *FlapsClient) DeleteACMECertificate(ctx context.Context, appName, hostname string) error {
	panic("TODO")
}

func (m *FlapsClient) DeleteCertificate(ctx context.Context, appName, hostname string) error {
	panic("TODO")
}

func (m *FlapsClient) DeleteCustomCertificate(ctx context.Context, appName, hostname string) error {
	panic("TODO")
}

func (m *FlapsClient) DeleteMetadata(ctx context.Context, appName, machineID, key string) error {
	panic("TODO")
}

func (m *FlapsClient) DeleteAppSecret(ctx context.Context, appName, name string) (*fly.DeleteAppSecretResp, error) {
	panic("TODO")
}

func (m *FlapsClient) DeleteIPAssignment(ctx context.Context, appName, ip string) (err error) {
	panic("TODO")
}

func (m *FlapsClient) DeleteSecretKey(ctx context.Context, appName, name string) error {
	panic("TODO")
}

func (m *FlapsClient) DeleteVolume(ctx context.Context, appName, volumeId string) (*fly.Volume, error) {
	panic("TODO")
}

func (m *FlapsClient) Destroy(ctx context.Context, appName string, input fly.RemoveMachineInput, nonce string) (err error) {
	panic("TODO")
}

func (m *FlapsClient) Exec(ctx context.Context, appName, machineID string, in *fly.MachineExecRequest) (*fly.MachineExecResponse, error) {
	panic("TODO")
}

func (m *FlapsClient) ExtendVolume(ctx context.Context, appName, volumeId string, size_gb int) (*fly.Volume, bool, error) {
	panic("TODO")
}

func (m *FlapsClient) FindLease(ctx context.Context, appName, machineID string) (*fly.MachineLease, error) {
	panic("TODO")
}

func (m *FlapsClient) GenerateSecretKey(ctx context.Context, appName, name string, typ string) (*fly.SetSecretKeyResp, error) {
	panic("TODO")
}

func (m *FlapsClient) Get(ctx context.Context, appName, machineID string) (*fly.Machine, error) {
	return m.server.GetMachine(ctx, appName, machineID)
}

func (m *FlapsClient) GetApp(ctx context.Context, name string) (*flaps.App, error) {
	m.server.mu.Lock()
	defer m.server.mu.Unlock()

	app := m.server.apps[name]
	if app == nil {
		return nil, fmt.Errorf("app not found: %q", name)
	}

	return &flaps.App{
		Name: app.Name,
		Organization: flaps.AppOrganizationInfo{
			Slug: app.Organization.Slug,
		},
	}, nil
}

func (m *FlapsClient) GetAllVolumes(ctx context.Context, appName string) ([]fly.Volume, error) {
	panic("TODO")
}

func (m *FlapsClient) GetCertificate(ctx context.Context, appName, hostname string) (*fly.CertificateDetailResponse, error) {
	panic("TODO")
}

func (m *FlapsClient) GetIPAssignments(ctx context.Context, appName string) (res *flaps.ListIPAssignmentsResponse, err error) {
	return &flaps.ListIPAssignmentsResponse{}, nil
}

func (m *FlapsClient) GetMany(ctx context.Context, appName string, machineIDs []string) ([]*fly.Machine, error) {
	panic("TODO")
}

func (m *FlapsClient) GetMetadata(ctx context.Context, appName, machineID string) (map[string]string, error) {
	panic("TODO")
}

func (m *FlapsClient) GetPlacements(ctx context.Context, req *flaps.GetPlacementsRequest) ([]flaps.RegionPlacement, error) {
	panic("TODO")
}

func (m *FlapsClient) GetProcesses(ctx context.Context, appName, machineID string) (fly.MachinePsResponse, error) {
	return nil, nil // TODO
}

func (m *FlapsClient) GetRegions(ctx context.Context) (*flaps.RegionData, error) {
	panic("TODO")
}

func (m *FlapsClient) GetVolume(ctx context.Context, appName, volumeId string) (*fly.Volume, error) {
	panic("TODO")
}

func (m *FlapsClient) GetVolumeSnapshots(ctx context.Context, appName, volumeId string) ([]fly.VolumeSnapshot, error) {
	panic("TODO")
}

func (m *FlapsClient) GetVolumes(ctx context.Context, appName string) ([]fly.Volume, error) {
	panic("TODO")
}

func (m *FlapsClient) CreateCustomCertificate(ctx context.Context, appName string, req fly.ImportCertificateRequest) (*fly.CertificateDetailResponse, error) {
	panic("TODO")
}

func (m *FlapsClient) Kill(ctx context.Context, appName, machineID string) (err error) {
	panic("TODO")
}

func (m *FlapsClient) Launch(ctx context.Context, appName string, builder fly.LaunchMachineInput) (out *fly.Machine, err error) {
	return m.server.Launch(ctx, appName, builder.Name, builder.Region, builder.Config)
}

func (m *FlapsClient) List(ctx context.Context, appName, state string) ([]*fly.Machine, error) {
	panic("TODO")
}

func (m *FlapsClient) ListActive(ctx context.Context, appName string) ([]*fly.Machine, error) {
	m.server.mu.Lock()
	defer m.server.mu.Unlock()

	var a []*fly.Machine
	for _, machine := range m.server.machines[appName] {
		if !machine.IsReleaseCommandMachine() && !machine.IsFlyAppsConsole() && machine.IsActive() {
			a = append(a, machine)
		}
	}
	return a, nil
}

func (m *FlapsClient) ListApps(ctx context.Context, req flaps.ListAppsRequest) ([]flaps.App, error) {
	panic("TODO")
}

func (m *FlapsClient) ListAppSecrets(ctx context.Context, appName string, version *uint64, showSecrets bool) ([]fly.AppSecret, error) {
	panic("TODO")
}

func (m *FlapsClient) ListCertificates(ctx context.Context, appName string, opts *flaps.ListCertificatesOpts) (*fly.ListCertificatesResponse, error) {
	panic("TODO")
}

func (m *FlapsClient) ListFlyAppsMachines(ctx context.Context, appName string) (machines []*fly.Machine, releaseCmdMachine *fly.Machine, err error) {
	m.server.mu.Lock()
	defer m.server.mu.Unlock()

	machines = make([]*fly.Machine, 0)
	for _, machine := range m.server.machines[appName] {
		if machine.IsFlyAppsPlatform() && machine.IsActive() && !machine.IsFlyAppsReleaseCommand() && !machine.IsFlyAppsConsole() {
			machines = append(machines, machine)
		} else if machine.IsFlyAppsReleaseCommand() {
			releaseCmdMachine = machine
		}
	}
	return machines, releaseCmdMachine, nil
}

func (m *FlapsClient) ListSecretKeys(ctx context.Context, appName string, version *uint64) ([]fly.SecretKey, error) {
	panic("TODO")
}

func (m *FlapsClient) NewRequest(ctx context.Context, method, path string, in interface{}, headers map[string][]string) (*http.Request, error) {
	panic("TODO")
}

func (m *FlapsClient) RefreshLease(ctx context.Context, appName, machineID string, ttl *int, nonce string) (*fly.MachineLease, error) {
	panic("TODO")
}

func (m *FlapsClient) ReleaseLease(ctx context.Context, appName, machineID, nonce string) error {
	panic("TODO")
}

func (m *FlapsClient) Restart(ctx context.Context, appName string, in fly.RestartMachineInput, nonce string) (err error) {
	panic("TODO")
}

func (m *FlapsClient) SetMetadata(ctx context.Context, appName, machineID, key, value string) error {
	panic("TODO")
}

func (m *FlapsClient) SetAppSecret(ctx context.Context, appName, name string, value string) (*fly.SetAppSecretResp, error) {
	panic("TODO")
}

func (m *FlapsClient) SetSecretKey(ctx context.Context, appName, name string, typ string, value []byte) (*fly.SetSecretKeyResp, error) {
	panic("TODO")
}

func (m *FlapsClient) Start(ctx context.Context, appName, machineID string, nonce string) (out *fly.MachineStartResponse, err error) {
	panic("TODO")
}

func (m *FlapsClient) Stop(ctx context.Context, appName string, in fly.StopMachineInput, nonce string) (err error) {
	panic("TODO")
}

func (m *FlapsClient) Suspend(ctx context.Context, appName, machineID, nonce string) error {
	panic("TODO")
}

func (m *FlapsClient) Uncordon(ctx context.Context, appName, machineID string, nonce string) (err error) {
	panic("TODO")
}

func (m *FlapsClient) Update(ctx context.Context, appName string, builder fly.LaunchMachineInput, nonce string) (out *fly.Machine, err error) {
	panic("TODO")
}

func (m *FlapsClient) UpdateAppSecrets(ctx context.Context, appName string, values map[string]*string) (*fly.UpdateAppSecretsResp, error) {
	panic("TODO")
}

func (m *FlapsClient) UpdateVolume(ctx context.Context, appName, volumeId string, req fly.UpdateVolumeRequest) (*fly.Volume, error) {
	panic("TODO")
}

func (m *FlapsClient) Wait(ctx context.Context, appName string, machine *fly.Machine, state string, timeout time.Duration) (err error) {
	if state == "" {
		state = "started"
	}
	mach, err := m.server.GetMachine(ctx, appName, machine.ID)
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
