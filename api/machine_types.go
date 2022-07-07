package api

import (
	"fmt"
	"time"
)

type Machine struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	State string `json:"state"`

	Region string `json:"region"`

	ImageRef machineImageRef `json:"image_ref"`

	// InstanceID is unique for each version of the machine
	InstanceID string `json:"instance_id"`

	// PrivateIP is the internal 6PN address of the machine.
	PrivateIP string `json:"private_ip"`

	CreatedAt string `json:"created_at"`

	UpdatedAt string `json:"updated_at"`

	Config *MachineConfig `json:"config"`

	Events     []*MachineEvent `json:"events,omitempty"`
	LeaseNonce string
}

func (m Machine) FullImageRef() string {
	return fmt.Sprintf("%s:%s", m.ImageRef.Repository, m.ImageRef.Tag)
}

type machineImageRef struct {
	Registry   string            `json:"registry"`
	Repository string            `json:"repository"`
	Tag        string            `json:"tag"`
	Digest     string            `json:"digest"`
	Labels     map[string]string `json:"labels"`
}

type MachineEvent struct {
	Type      string          `json:"type"`
	Status    string          `json:"status"`
	Request   *MachineRequest `json:"request,omitempty"`
	Source    string          `json:"source"`
	Timestamp int64           `json:"timestamp"`
}

type MachineRequest struct {
	ExitEvent    *MachineExitEvent `json:"exit_event,omitempty"`
	RestartCount int64             `json:"restart_count"`
}
type MachineExitEvent struct {
	ExitCode      int16 `json:"exit_code"`
	GuestExitCode int16 `json:"guest_exit_code"`
	GuestSignal   int16 `json:"guest_signal"`
	OOMKilled     bool  `json:"oom_killed"`
	RequestedStop bool  `json:"requested_stop"`
	Resarting     bool  `json:"restarting"`
	Signal        int16 `json:"signal"`
}

type StopMachineInput struct {
	ID      string        `json:"id"`
	Signal  Signal        `json:"signal,omitempty"`
	Timeout time.Duration `json:"timeout,omitempty"`
	Filters *Filters      `json:"filters,omitempty"`
}

type MachineIP struct {
	Family   string
	Kind     string
	IP       string
	MaskSize int
}

type RemoveMachineInput struct {
	AppID string `json:"appId,omitempty"`
	ID    string `json:"id"`

	Kill bool `json:"kill"`
}

type MachineRestartPolicy string

var MachineRestartPolicyNo MachineRestartPolicy = "no"
var MachineRestartPolicyOnFailure MachineRestartPolicy = "on-failure"
var MachineRestartPolicyAlways MachineRestartPolicy = "always"

type MachineRestart struct {
	Policy MachineRestartPolicy `json:"policy"`
	// MaxRetries is only relevant with the on-failure policy.
	MaxRetries int `json:"max_retries,omitempty"`
}

type MachineMount struct {
	Encrypted bool   `json:"encrypted"`
	Path      string `json:"path"`
	SizeGb    int    `json:"size_gb"`
	Volume    string `json:"volume"`
}

type MachineGuest struct {
	CPUKind  string `json:"cpu_kind"`
	CPUs     int    `json:"cpus"`
	MemoryMB int    `json:"memory_mb"`

	KernelArgs []string `json:"kernel_args,omitempty"`
}

const (
	MEMORY_MB_PER_SHARED_CPU = 256
	MEMORY_MB_PER_CPU        = 2048
)

var MachinePresets map[string]*MachineGuest = map[string]*MachineGuest{
	"shared-cpu-1x":    {CPUKind: "shared", CPUs: 1, MemoryMB: 1 * MEMORY_MB_PER_SHARED_CPU},
	"shared-cpu-2x":    {CPUKind: "shared", CPUs: 2, MemoryMB: 2 * MEMORY_MB_PER_SHARED_CPU},
	"shared-cpu-4x":    {CPUKind: "shared", CPUs: 4, MemoryMB: 4 * MEMORY_MB_PER_SHARED_CPU},
	"shared-cpu-8x":    {CPUKind: "shared", CPUs: 8, MemoryMB: 8 * MEMORY_MB_PER_SHARED_CPU},
	"dedicated-cpu-1x": {CPUKind: "dedicated", CPUs: 1, MemoryMB: 1 * MEMORY_MB_PER_CPU},
	"dedicated-cpu-2x": {CPUKind: "dedicated", CPUs: 2, MemoryMB: 2 * MEMORY_MB_PER_CPU},
	"dedicated-cpu-4x": {CPUKind: "dedicated", CPUs: 4, MemoryMB: 4 * MEMORY_MB_PER_CPU},
	"dedicated-cpu-8x": {CPUKind: "dedicated", CPUs: 8, MemoryMB: 8 * MEMORY_MB_PER_CPU},
}

type MachineMetrics struct {
	Port int    `toml:"port" json:"port"`
	Path string `toml:"path" json:"path"`
}

type MachinePort struct {
	Port       int      `json:"port" toml:"port"`
	Handlers   []string `json:"handlers,omitempty" toml:"handlers,omitempty"`
	ForceHttps bool     `json:"force_https,omitempty" toml:"force_https,omitempty"`
}

type MachineService struct {
	Protocol     string        `json:"protocol" toml:"protocol"`
	InternalPort int           `json:"internal_port" toml:"internal_port"`
	Ports        []MachinePort `json:"ports" toml:"ports"`
}

type MachineConfig struct {
	Env      map[string]string `json:"env"`
	Init     MachineInit       `json:"init,omitempty"`
	Image    string            `json:"image"`
	ImageRef machineImageRef   `json:"image_ref"`
	Metadata map[string]string `json:"metadata"`
	Mounts   []MachineMount    `json:"mounts,omitempty"`
	Restart  MachineRestart    `json:"restart,omitempty"`
	Services []MachineService  `json:"services,omitempty"`
	VMSize   string            `json:"size,omitempty"`
	Guest    *MachineGuest     `json:"guest,omitempty"`
	Metrics  *MachineMetrics   `json:"metrics"`
}

type MachineLease struct {
	Status string `json:"status"`
	Data   struct {
		Nonce     string `json:"nonce"`
		ExpiresAt int64  `json:"expires_at"`
		Owner     string `json:"owner"`
	}
}

type MachineStartResponse struct {
	Message       string `json:"message,omitempty"`
	Status        string `json:"status,omitempty"`
	PreviousState string `json:"previous_state"`
}
