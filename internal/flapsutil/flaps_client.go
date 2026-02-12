package flapsutil

import (
	"context"
	"net/http"
	"time"

	fly "github.com/superfly/fly-go"
	"github.com/superfly/fly-go/flaps"
)

var _ FlapsClient = (*flaps.Client)(nil)

type FlapsClient interface {
	AcquireLease(ctx context.Context, appName, machineID string, ttl *int) (*fly.MachineLease, error)
	AssignIP(ctx context.Context, appName string, req flaps.AssignIPRequest) (res *flaps.IPAssignment, err error)
	CheckCertificate(ctx context.Context, appName, hostname string) (*fly.CertificateDetailResponse, error)
	Cordon(ctx context.Context, appName, machineID string, nonce string) (err error)
	CreateApp(ctx context.Context, req flaps.CreateAppRequest) (*flaps.App, error)
	CreateACMECertificate(ctx context.Context, appName string, req fly.CreateCertificateRequest) (*fly.CertificateDetailResponse, error)
	CreateVolume(ctx context.Context, appName string, req fly.CreateVolumeRequest) (*fly.Volume, error)
	CreateVolumeSnapshot(ctx context.Context, appName, volumeId string) error
	DeleteApp(ctx context.Context, name string) error
	DeleteACMECertificate(ctx context.Context, appName, hostname string) error
	DeleteCertificate(ctx context.Context, appName, hostname string) error
	DeleteCustomCertificate(ctx context.Context, appName, hostname string) error
	DeleteMetadata(ctx context.Context, appName, machineID, key string) error
	DeleteAppSecret(ctx context.Context, appName, name string) (*fly.DeleteAppSecretResp, error)
	DeleteIPAssignment(ctx context.Context, appName, ip string) (err error)
	DeleteSecretKey(ctx context.Context, appName, name string) error
	DeleteVolume(ctx context.Context, appName, volumeId string) (*fly.Volume, error)
	Destroy(ctx context.Context, appName string, input fly.RemoveMachineInput, nonce string) (err error)
	Exec(ctx context.Context, appName, machineID string, in *fly.MachineExecRequest) (*fly.MachineExecResponse, error)
	ExtendVolume(ctx context.Context, appName, volumeId string, size_gb int) (*fly.Volume, bool, error)
	FindLease(ctx context.Context, appName, machineID string) (*fly.MachineLease, error)
	GenerateSecretKey(ctx context.Context, appName, name string, typ string) (*fly.SetSecretKeyResp, error)
	Get(ctx context.Context, appName, machineID string) (*fly.Machine, error)
	GetApp(ctx context.Context, name string) (app *flaps.App, err error)
	GetAllVolumes(ctx context.Context, appName string) ([]fly.Volume, error)
	GetCertificate(ctx context.Context, appName, hostname string) (*fly.CertificateDetailResponse, error)
	GetIPAssignments(ctx context.Context, appName string) (res *flaps.ListIPAssignmentsResponse, err error)
	GetMany(ctx context.Context, appName string, machineIDs []string) ([]*fly.Machine, error)
	GetMetadata(ctx context.Context, appName, machineID string) (map[string]string, error)
	GetPlacements(ctx context.Context, req *flaps.GetPlacementsRequest) ([]flaps.RegionPlacement, error)
	GetProcesses(ctx context.Context, appName, machineID string) (fly.MachinePsResponse, error)
	GetRegions(ctx context.Context) (*flaps.RegionData, error)
	GetVolume(ctx context.Context, appName, volumeId string) (*fly.Volume, error)
	GetVolumeSnapshots(ctx context.Context, appName, volumeId string) ([]fly.VolumeSnapshot, error)
	GetVolumes(ctx context.Context, appName string) ([]fly.Volume, error)
	CreateCustomCertificate(ctx context.Context, appName string, req fly.ImportCertificateRequest) (*fly.CertificateDetailResponse, error)
	Kill(ctx context.Context, appName, machineID string) (err error)
	Launch(ctx context.Context, appName string, builder fly.LaunchMachineInput) (out *fly.Machine, err error)
	List(ctx context.Context, appName, state string) ([]*fly.Machine, error)
	ListActive(ctx context.Context, appName string) ([]*fly.Machine, error)
	ListApps(ctx context.Context, req flaps.ListAppsRequest) ([]flaps.App, error)
	ListAppSecrets(ctx context.Context, appName string, version *uint64, showSecrets bool) ([]fly.AppSecret, error)
	ListCertificates(ctx context.Context, appName string, opts *flaps.ListCertificatesOpts) (*fly.ListCertificatesResponse, error)
	ListFlyAppsMachines(ctx context.Context, appName string) ([]*fly.Machine, *fly.Machine, error)
	ListSecretKeys(ctx context.Context, appName string, version *uint64) ([]fly.SecretKey, error)
	NewRequest(ctx context.Context, method, path string, in interface{}, headers map[string][]string) (*http.Request, error)
	RefreshLease(ctx context.Context, appName, machineID string, ttl *int, nonce string) (*fly.MachineLease, error)
	ReleaseLease(ctx context.Context, appName, machineID, nonce string) error
	Restart(ctx context.Context, appName string, in fly.RestartMachineInput, nonce string) (err error)
	SetAppSecret(ctx context.Context, appName, name string, value string) (*fly.SetAppSecretResp, error)
	SetSecretKey(ctx context.Context, appName, name string, typ string, value []byte) (*fly.SetSecretKeyResp, error)
	SetMetadata(ctx context.Context, appName, machineID, key, value string) error
	Start(ctx context.Context, appName, machineID string, nonce string) (out *fly.MachineStartResponse, err error)
	Stop(ctx context.Context, appName string, in fly.StopMachineInput, nonce string) (err error)
	Suspend(ctx context.Context, appName, machineID, nonce string) error
	Uncordon(ctx context.Context, appName, machineID string, nonce string) (err error)
	Update(ctx context.Context, appName string, builder fly.LaunchMachineInput, nonce string) (out *fly.Machine, err error)
	UpdateAppSecrets(ctx context.Context, appName string, values map[string]*string) (*fly.UpdateAppSecretsResp, error)
	UpdateVolume(ctx context.Context, appName, volumeId string, req fly.UpdateVolumeRequest) (*fly.Volume, error)
	Wait(ctx context.Context, appName string, machine *fly.Machine, state string, timeout time.Duration) (err error)
	WaitForApp(ctx context.Context, name string) error
}

type contextKey struct{}

var clientContextKey = &contextKey{}

// NewContext derives a context that carries c from ctx.
func NewContextWithClient(ctx context.Context, c FlapsClient) context.Context {
	return context.WithValue(ctx, clientContextKey, c)
}

// ClientFromContext returns the client ctx carries.
func ClientFromContext(ctx context.Context) FlapsClient {
	c, _ := ctx.Value(clientContextKey).(FlapsClient)
	return c
}
