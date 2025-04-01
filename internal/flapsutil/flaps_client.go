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
	AcquireLease(ctx context.Context, machineID string, ttl *int) (*fly.MachineLease, error)
	Cordon(ctx context.Context, machineID string, nonce string) (err error)
	CreateApp(ctx context.Context, name string, org string) (err error)
	CreateSecret(ctx context.Context, sLabel, sType string, in fly.CreateSecretRequest) (err error)
	CreateVolume(ctx context.Context, req fly.CreateVolumeRequest) (*fly.Volume, error)
	CreateVolumeSnapshot(ctx context.Context, volumeId string) error
	DeleteMetadata(ctx context.Context, machineID, key string) error
	DeleteSecret(ctx context.Context, label string) (err error)
	DeleteVolume(ctx context.Context, volumeId string) (*fly.Volume, error)
	Destroy(ctx context.Context, input fly.RemoveMachineInput, nonce string) (err error)
	Exec(ctx context.Context, machineID string, in *fly.MachineExecRequest) (*fly.MachineExecResponse, error)
	ExtendVolume(ctx context.Context, volumeId string, size_gb int) (*fly.Volume, bool, error)
	FindLease(ctx context.Context, machineID string) (*fly.MachineLease, error)
	GenerateSecret(ctx context.Context, sLabel, sType string) (err error)
	Get(ctx context.Context, machineID string) (*fly.Machine, error)
	GetAllVolumes(ctx context.Context) ([]fly.Volume, error)
	GetMany(ctx context.Context, machineIDs []string) ([]*fly.Machine, error)
	GetMetadata(ctx context.Context, machineID string) (map[string]string, error)
	GetProcesses(ctx context.Context, machineID string) (fly.MachinePsResponse, error)
	GetVolume(ctx context.Context, volumeId string) (*fly.Volume, error)
	GetVolumeSnapshots(ctx context.Context, volumeId string) ([]fly.VolumeSnapshot, error)
	GetVolumes(ctx context.Context) ([]fly.Volume, error)
	Kill(ctx context.Context, machineID string) (err error)
	Launch(ctx context.Context, builder fly.LaunchMachineInput) (out *fly.Machine, err error)
	List(ctx context.Context, state string) ([]*fly.Machine, error)
	ListActive(ctx context.Context) ([]*fly.Machine, error)
	ListFlyAppsMachines(ctx context.Context) ([]*fly.Machine, *fly.Machine, error)
	ListSecrets(ctx context.Context) (out []fly.ListSecret, err error)
	NewRequest(ctx context.Context, method, path string, in interface{}, headers map[string][]string) (*http.Request, error)
	RefreshLease(ctx context.Context, machineID string, ttl *int, nonce string) (*fly.MachineLease, error)
	ReleaseLease(ctx context.Context, machineID, nonce string) error
	Restart(ctx context.Context, in fly.RestartMachineInput, nonce string) (err error)
	SetMetadata(ctx context.Context, machineID, key, value string) error
	Start(ctx context.Context, machineID string, nonce string) (out *fly.MachineStartResponse, err error)
	Stop(ctx context.Context, in fly.StopMachineInput, nonce string) (err error)
	Suspend(ctx context.Context, machineID, nonce string) error
	Uncordon(ctx context.Context, machineID string, nonce string) (err error)
	Update(ctx context.Context, builder fly.LaunchMachineInput, nonce string) (out *fly.Machine, err error)
	UpdateVolume(ctx context.Context, volumeId string, req fly.UpdateVolumeRequest) (*fly.Volume, error)
	Wait(ctx context.Context, machine *fly.Machine, state string, timeout time.Duration) (err error)
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
