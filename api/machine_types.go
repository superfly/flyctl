package api

import (
	"fmt"
	"time"
)

type Machine struct {
	ID       string          `json:"id"`
	Name     string          `json:"name"`
	State    string          `json:"state"`
	Region   string          `json:"region"`
	ImageRef machineImageRef `json:"image_ref"`
	// InstanceID is unique for each version of the machine
	InstanceID string `json:"instance_id"`
	Version    string `json:"version"`
	// PrivateIP is the internal 6PN address of the machine.
	PrivateIP  string                `json:"private_ip"`
	CreatedAt  string                `json:"created_at"`
	UpdatedAt  string                `json:"updated_at"`
	Config     *MachineConfig        `json:"config"`
	Events     []*MachineEvent       `json:"events,omitempty"`
	Checks     []*MachineCheckStatus `json:"checks,omitempty"`
	LeaseNonce string
}

func (m Machine) FullImageRef() string {
	return fmt.Sprintf("%s/%s:%s", m.ImageRef.Registry, m.ImageRef.Repository, m.ImageRef.Tag)
}

func (m Machine) ImageRefWithVersion() string {
	ref := fmt.Sprintf("%s:%s", m.ImageRef.Repository, m.ImageRef.Tag)
	version := m.ImageRef.Labels["fly.version"]
	if version != "" {
		ref = fmt.Sprintf("%s (%s)", ref, version)
	}

	return ref
}

func (m Machine) ImageVersion() string {
	if m.ImageRef.Labels == nil {
		return ""
	}
	return m.ImageRef.Labels["fly.version"]
}

func (m Machine) ImageRepository() string {
	return m.ImageRef.Repository
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

type RestartMachineInput struct {
	ID               string        `json:"id"`
	Signal           *Signal       `json:"signal,omitempty"`
	Timeout          time.Duration `json:"timeout,omitempty"`
	ForceStop        bool          `json:"force_stop,omitempty"`
	SkipHealthChecks bool          `json:"skip_health_checks,omitempty"`
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

var (
	MachineRestartPolicyNo        MachineRestartPolicy = "no"
	MachineRestartPolicyOnFailure MachineRestartPolicy = "on-failure"
	MachineRestartPolicyAlways    MachineRestartPolicy = "always"
)

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
	Name      string `json:"name"`
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

// TODO - Determine if we want allocate max memory allocation, or minimum per # cpus.
var MachinePresets map[string]*MachineGuest = map[string]*MachineGuest{
	"shared-cpu-1x": {CPUKind: "shared", CPUs: 1, MemoryMB: 1 * MEMORY_MB_PER_SHARED_CPU},
	"shared-cpu-2x": {CPUKind: "shared", CPUs: 2, MemoryMB: 2 * MEMORY_MB_PER_SHARED_CPU},
	"shared-cpu-4x": {CPUKind: "shared", CPUs: 4, MemoryMB: 4 * MEMORY_MB_PER_SHARED_CPU},
	"shared-cpu-8x": {CPUKind: "shared", CPUs: 8, MemoryMB: 8 * MEMORY_MB_PER_SHARED_CPU},
}

type MachineMetrics struct {
	Port int    `toml:"port" json:"port"`
	Path string `toml:"path" json:"path"`
}

type MachineCheck struct {
	Type       string    `json:"type,omitempty"`
	Port       uint16    `json:"port,omitempty"`
	Interval   *Duration `json:"interval,omitempty" toml:",omitempty"`
	Timeout    *Duration `json:"timeout,omitempty" toml:",omitempty"`
	HTTPMethod *string   `json:"method,omitempty" toml:"method,omitempty"`
	HTTPPath   *string   `json:"path,omitempty" toml:"path,omitempty"`
}

type MachineCheckStatus struct {
	Name      string     `json:"name,omitempty"`
	Status    string     `json:"status,omitempty"`
	Output    string     `json:"output,omitempty"`
	UpdatedAt *time.Time `json:"updated_at,omitempty"`
}

type MachinePort struct {
	Port       int      `json:"port" toml:"port"`
	Handlers   []string `json:"handlers,omitempty" toml:"handlers,omitempty"`
	ForceHttps bool     `json:"force_https,omitempty" toml:"force_https,omitempty"`
}

type MachineService struct {
	Protocol     string                     `json:"protocol" toml:"protocol"`
	InternalPort int                        `json:"internal_port" toml:"internal_port"`
	Ports        []MachinePort              `json:"ports" toml:"ports"`
	Concurrency  *MachineServiceConcurrency `json:"concurrency,omitempty" toml:"concurrency"`
}

type MachineServiceConcurrency struct {
	Type      string `json:"type" toml:"type,omitempty"`
	HardLimit int    `json:"hard_limit" toml:"hard_limit,omitempty"`
	SoftLimit int    `json:"soft_limit" toml:"soft_limit,omitempty"`
}

type MachineConfig struct {
	Env       map[string]string       `json:"env"`
	Init      MachineInit             `json:"init,omitempty"`
	Processes []MachineProcess        `json:"processes,omitempty"`
	Image     string                  `json:"image"`
	Metadata  map[string]string       `json:"metadata"`
	Mounts    []MachineMount          `json:"mounts,omitempty"`
	Restart   MachineRestart          `json:"restart,omitempty"`
	Services  []MachineService        `json:"services,omitempty"`
	VMSize    string                  `json:"size,omitempty"`
	Guest     *MachineGuest           `json:"guest,omitempty"`
	Metrics   *MachineMetrics         `json:"metrics"`
	Schedule  string                  `json:"schedule,omitempty"`
	Checks    map[string]MachineCheck `json:"checks,omitempty"`
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

type LaunchMachineInput struct {
	AppID   string         `json:"appId,omitempty"`
	ID      string         `json:"id,omitempty"`
	Name    string         `json:"name,omitempty"`
	OrgSlug string         `json:"organizationId,omitempty"`
	Region  string         `json:"region,omitempty"`
	Config  *MachineConfig `json:"config"`
	// Client side only
	SkipHealthChecks bool
}

type MachineProcess struct {
	ExecOverride       []string          `json:"exec,omitempty"`
	EntrypointOverride []string          `json:"entrypoint,omitempty"`
	CmdOverride        []string          `json:"cmd,omitempty"`
	UserOverride       string            `json:"user,omitempty"`
	ExtraEnv           map[string]string `json:"env"`
}
