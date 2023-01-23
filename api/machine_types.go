package api

import (
	"fmt"
	"time"
)

const (
	MachineConfigMetadataKeyFlyManagedPostgres = "fly-managed-postgres"
	MachineConfigMetadataKeyFlyPlatformVersion = "fly_platform_version"
	MachineConfigMetadataKeyFlyReleaseId       = "fly_release_id"
	MachineConfigMetadataKeyFlyReleaseVersion  = "fly_release_version"
	MachineConfigMetadataKeyFlyProcessGroup    = "fly_process_group"
	MachineFlyPlatformVersion2                 = "v2"
	MachineProcessGroupApp                     = "app"
	MachineProcessGroupFlyAppReleaseCommand    = "fly_app_release_command"
	MachineStateDestroyed                      = "destroyed"
	MachineStateDestroying                     = "destroying"
	MachineStateStarted                        = "started"
	MachineStateStopped                        = "stopped"
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

func (m *Machine) IsFlyAppsPlatform() bool {
	return m.Config != nil && m.Config.Metadata[MachineConfigMetadataKeyFlyPlatformVersion] == MachineFlyPlatformVersion2 && m.IsActive()
}

func (m *Machine) IsFlyAppsReleaseCommand() bool {
	return m.IsFlyAppsPlatform() && m.HasProcessGroup(MachineProcessGroupFlyAppReleaseCommand)
}

func (m *Machine) IsActive() bool {
	return m.State != MachineStateDestroyed && m.State != MachineStateDestroying
}

func (m *Machine) HasProcessGroup(desired string) bool {
	return m.Config != nil && m.Config.Metadata[MachineConfigMetadataKeyFlyProcessGroup] == desired
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

type HealthCheckStatus struct {
	Total, Passing, Warn, Critical int
}

func (hcs *HealthCheckStatus) AllPassing() bool {
	return hcs.Passing == hcs.Total
}

func (m *Machine) HealthCheckStatus() *HealthCheckStatus {
	res := &HealthCheckStatus{}
	res.Total = len(m.Checks)
	for _, check := range m.Checks {
		switch check.Status {
		case "passing":
			res.Passing += 1
		case "warn":
			res.Warn += 1
		case "critical":
			res.Critical += 1
		}
	}
	return res
}

// Finds the latest event of type latestEventType, which happened after the most recent event of type firstEventType
func (m *Machine) GetLatestEventOfTypeAfterType(latestEventType, firstEventType string) *MachineEvent {
	firstIndex := 0
	for i, e := range m.Events {
		if e.Type == firstEventType {
			firstIndex = i
			break
		}
	}
	for _, e := range m.Events[0:firstIndex] {
		if e.Type == latestEventType {
			return e
		}
	}
	return nil
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
	ExitEvent    *MachineExitEvent    `json:"exit_event,omitempty"`
	MonitorEvent *MachineMonitorEvent `json:"MonitorEvent,omitempty"`
	RestartCount int                  `json:"restart_count"`
}

// returns the ExitCode from MonitorEvent if it exists, otherwise ExitEvent
// error when MonitorEvent and ExitEvent are both nil
func (mr *MachineRequest) GetExitCode() (int, error) {
	if mr.MonitorEvent != nil && mr.MonitorEvent.ExitEvent != nil {
		return mr.MonitorEvent.ExitEvent.ExitCode, nil
	} else if mr.ExitEvent != nil {
		return mr.MonitorEvent.ExitEvent.ExitCode, nil
	} else {
		return -1, fmt.Errorf("error no exit code in this MachineRequest")
	}
}

type MachineMonitorEvent struct {
	ExitEvent *MachineExitEvent `json:"exit_event,omitempty"`
}

type MachineExitEvent struct {
	ExitCode      int       `json:"exit_code,omitempty"`
	GuestExitCode int       `json:"guest_exit_code,omitempty"`
	GuestSignal   int       `json:"guest_signal,omitempty"`
	OOMKilled     bool      `json:"oom_killed,omitempty"`
	RequestedStop bool      `json:"requested_stop,omitempty"`
	Restarting    bool      `json:"restarting,omitempty"`
	Signal        int       `json:"signal,omitempty"`
	ExitedAt      time.Time `json:"exited_at,omitempty"`
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
	Type              string              `json:"type,omitempty"`
	Port              uint16              `json:"port,omitempty"`
	Interval          *Duration           `json:"interval,omitempty"`
	Timeout           *Duration           `json:"timeout,omitempty"`
	GracePeriod       *uint64             `json:"grace_period,omitempty"`
	RestartLimit      *uint64             `json:"restart_limit,omitempty"`
	HTTPMethod        *string             `json:"method,omitempty"`
	HTTPPath          *string             `json:"path,omitempty"`
	HTTPProtocol      *string             `json:"protocol,omitempty"`
	HTTPSkipTLSVerify *bool               `json:"tls_skip_verify,omitempty"`
	HTTPHeaders       map[string][]string `json:"headers,omitempty"`
}

type MachineCheckStatus struct {
	Name      string     `json:"name,omitempty"`
	Status    string     `json:"status,omitempty"`
	Output    string     `json:"output,omitempty"`
	UpdatedAt *time.Time `json:"updated_at,omitempty"`
}

type MachinePort struct {
	Port       *int     `json:"port,omitempty" toml:"port,omitempty"`
	StartPort  *int     `json:"start_port,omitempty" toml:"start_port,omitempty"`
	EndPort    *int     `json:"end_port,omitempty" toml:"end_port,omitempty"`
	Handlers   []string `json:"handlers,omitempty" toml:"handlers,omitempty"`
	ForceHttps bool     `json:"force_https,omitempty" toml:"force_https,omitempty"`
}

func (mp *MachinePort) ContainsPort(port int) bool {
	if mp.Port != nil && port == *mp.Port {
		return true
	}
	if mp.StartPort == nil && mp.EndPort == nil {
		return false
	}
	startPort := 0
	endPort := 65535
	if mp.StartPort != nil {
		startPort = *mp.StartPort
	}
	if mp.EndPort != nil {
		endPort = *mp.EndPort
	}
	return startPort <= port && port <= endPort
}

func (mp *MachinePort) HasNonHttpPorts() bool {
	if mp.Port != nil && *mp.Port != 443 && *mp.Port != 80 {
		return true
	}
	if mp.StartPort == nil && mp.EndPort == nil {
		return false
	}
	startPort := 0
	endPort := 65535
	if mp.StartPort != nil {
		startPort = *mp.StartPort
	}
	if mp.EndPort != nil {
		endPort = *mp.EndPort
	}
	portRangeCount := endPort - startPort + 1
	if portRangeCount > 2 {
		return true
	}
	httpInRange := startPort <= 80 && 80 <= endPort
	httpsInRange := startPort <= 443 && 443 <= endPort
	switch {
	case portRangeCount == 2:
		return !httpInRange || !httpsInRange
	case portRangeCount == 1:
		return !httpInRange && !httpsInRange
	}
	return false
}

type MachineService struct {
	Protocol     string                     `json:"protocol" toml:"protocol"`
	InternalPort int                        `json:"internal_port" toml:"internal_port"`
	Ports        []MachinePort              `json:"ports" toml:"ports"`
	Checks       []Check                    `json:"checks,omitempty" toml:"checks,omitempty"`
	Concurrency  *MachineServiceConcurrency `json:"concurrency,omitempty" toml:"concurrency"`
}

type MachineServiceConcurrency struct {
	Type      string `json:"type" toml:"type,omitempty"`
	HardLimit int    `json:"hard_limit" toml:"hard_limit,omitempty"`
	SoftLimit int    `json:"soft_limit" toml:"soft_limit,omitempty"`
}

type MachineConfig struct {
	Env         map[string]string       `json:"env"`
	Init        MachineInit             `json:"init,omitempty"`
	Processes   []MachineProcess        `json:"processes,omitempty"`
	Image       string                  `json:"image"`
	Metadata    map[string]string       `json:"metadata"`
	Mounts      []MachineMount          `json:"mounts,omitempty"`
	Restart     MachineRestart          `json:"restart,omitempty"`
	Services    []MachineService        `json:"services,omitempty"`
	VMSize      string                  `json:"size,omitempty"`
	Guest       *MachineGuest           `json:"guest,omitempty"`
	Metrics     *MachineMetrics         `json:"metrics"`
	Schedule    string                  `json:"schedule,omitempty"`
	Checks      map[string]MachineCheck `json:"checks,omitempty"`
	AutoDestroy bool                    `json:"auto_destroy"`
	DNS         *DNSConfig              `json:"dns,omitempty"`
}

type DNSConfig struct {
	SkipRegistration bool `json:"skip_registration"`
}

type MachineLease struct {
	Status  string            `json:"status"`
	Data    *MachineLeaseData `json:"data,omitempty"`
	Message string            `json:"message,omitempty"`
	Code    string            `json:"code,omitempty"`
}

type MachineLeaseData struct {
	Nonce     string `json:"nonce"`
	ExpiresAt int64  `json:"expires_at"`
	Owner     string `json:"owner"`
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

type MachineExecRequest struct {
	Cmd     string `json:"cmd"`
	Timeout int    `json:"timeout,omitempty"`
}

type MachineExecResponse struct {
	ExitCode int32   `json:"exit_code"`
	StdOut   *string `json:"stdout"`
	StdErr   *string `json:"stderr"`
}
