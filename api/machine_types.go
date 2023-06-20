package api

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

const (
	MachineConfigMetadataKeyFlyManagedPostgres = "fly-managed-postgres"
	MachineConfigMetadataKeyFlyPlatformVersion = "fly_platform_version"
	MachineConfigMetadataKeyFlyReleaseId       = "fly_release_id"
	MachineConfigMetadataKeyFlyReleaseVersion  = "fly_release_version"
	MachineConfigMetadataKeyFlyProcessGroup    = "fly_process_group"
	MachineConfigMetadataKeyFlyPreviousAlloc   = "fly_previous_alloc"
	MachineFlyPlatformVersion2                 = "v2"
	MachineProcessGroupApp                     = "app"
	MachineProcessGroupFlyAppReleaseCommand    = "fly_app_release_command"
	MachineProcessGroupFlyAppConsole           = "fly_app_console"
	MachineStateDestroyed                      = "destroyed"
	MachineStateDestroying                     = "destroying"
	MachineStateStarted                        = "started"
	MachineStateStopped                        = "stopped"
	MachineStateCreated                        = "created"
)

type Machine struct {
	ID       string          `json:"id,omitempty"`
	Name     string          `json:"name,omitempty"`
	State    string          `json:"state,omitempty"`
	Region   string          `json:"region,omitempty"`
	ImageRef MachineImageRef `json:"image_ref,omitempty"`
	// InstanceID is unique for each version of the machine
	InstanceID string `json:"instance_id,omitempty"`
	Version    string `json:"version,omitempty"`
	// PrivateIP is the internal 6PN address of the machine.
	PrivateIP  string                `json:"private_ip,omitempty"`
	CreatedAt  string                `json:"created_at,omitempty"`
	UpdatedAt  string                `json:"updated_at,omitempty"`
	Config     *MachineConfig        `json:"config,omitempty"`
	Events     []*MachineEvent       `json:"events,omitempty"`
	Checks     []*MachineCheckStatus `json:"checks,omitempty"`
	LeaseNonce string                `json:"nonce,omitempty"`
}

func (m *Machine) FullImageRef() string {
	imgStr := fmt.Sprintf("%s/%s", m.ImageRef.Registry, m.ImageRef.Repository)
	tag := m.ImageRef.Tag
	digest := m.ImageRef.Digest

	if tag != "" && digest != "" {
		imgStr = fmt.Sprintf("%s:%s@%s", imgStr, tag, digest)
	} else if digest != "" {
		imgStr = fmt.Sprintf("%s@%s", imgStr, digest)
	} else if tag != "" {
		imgStr = fmt.Sprintf("%s:%s", imgStr, tag)
	}

	return imgStr
}

func (m *Machine) ImageRefWithVersion() string {
	ref := fmt.Sprintf("%s:%s", m.ImageRef.Repository, m.ImageRef.Tag)
	version := m.ImageRef.Labels["fly.version"]
	if version != "" {
		ref = fmt.Sprintf("%s (%s)", ref, version)
	}

	return ref
}

func (m *Machine) IsAppsV2() bool {
	return m.Config != nil && m.Config.Metadata[MachineConfigMetadataKeyFlyPlatformVersion] == MachineFlyPlatformVersion2
}

func (m *Machine) IsFlyAppsPlatform() bool {
	return m.IsAppsV2() && m.IsActive()
}

func (m *Machine) IsFlyAppsReleaseCommand() bool {
	return m.IsFlyAppsPlatform() && m.IsReleaseCommandMachine()
}

func (m *Machine) IsFlyAppsConsole() bool {
	return m.IsFlyAppsPlatform() && m.HasProcessGroup(MachineProcessGroupFlyAppConsole)
}

func (m *Machine) IsActive() bool {
	return m.State != MachineStateDestroyed && m.State != MachineStateDestroying
}

func (m *Machine) ProcessGroup() string {
	if m.Config == nil {
		return ""
	}
	return m.Config.ProcessGroup()
}

func (m *Machine) HasProcessGroup(desired string) bool {
	return m.Config != nil && m.ProcessGroup() == desired
}

func (m *Machine) ImageVersion() string {
	if m.ImageRef.Labels == nil {
		return ""
	}
	return m.ImageRef.Labels["fly.version"]
}

func (m *Machine) ImageRepository() string {
	return m.ImageRef.Repository
}

func (m *Machine) TopLevelChecks() *HealthCheckStatus {
	res := &HealthCheckStatus{}
	total := 0

	for _, check := range m.Checks {
		if !strings.HasPrefix(check.Name, "servicecheck-") {
			total++
			switch check.Status {
			case Passing:
				res.Passing += 1
			case Warning:
				res.Warn += 1
			case Critical:
				res.Critical += 1
			}
		}
	}

	res.Total = total
	return res
}

type HealthCheckStatus struct {
	Total, Passing, Warn, Critical int
}

func (hcs *HealthCheckStatus) AllPassing() bool {
	return hcs.Passing == hcs.Total
}

func (m *Machine) AllHealthChecks() *HealthCheckStatus {
	res := &HealthCheckStatus{}
	res.Total = len(m.Checks)
	for _, check := range m.Checks {
		switch check.Status {
		case Passing:
			res.Passing += 1
		case Warning:
			res.Warn += 1
		case Critical:
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

func (m *Machine) IsReleaseCommandMachine() bool {
	return m.HasProcessGroup(MachineProcessGroupFlyAppReleaseCommand) || m.Config.Metadata["process_group"] == "release_command"
}

type MachineImageRef struct {
	Registry   string            `json:"registry,omitempty"`
	Repository string            `json:"repository,omitempty"`
	Tag        string            `json:"tag,omitempty"`
	Digest     string            `json:"digest,omitempty"`
	Labels     map[string]string `json:"labels,omitempty"`
}

type MachineEvent struct {
	Type      string          `json:"type,omitempty"`
	Status    string          `json:"status,omitempty"`
	Request   *MachineRequest `json:"request,omitempty"`
	Source    string          `json:"source,omitempty"`
	Timestamp int64           `json:"timestamp,omitempty"`
}

type MachineRequest struct {
	ExitEvent    *MachineExitEvent    `json:"exit_event,omitempty"`
	MonitorEvent *MachineMonitorEvent `json:"MonitorEvent,omitempty"`
	RestartCount int                  `json:"restart_count,omitempty"`
}

// returns the ExitCode from MonitorEvent if it exists, otherwise ExitEvent
// error when MonitorEvent and ExitEvent are both nil
func (mr *MachineRequest) GetExitCode() (int, error) {
	if mr.MonitorEvent != nil && mr.MonitorEvent.ExitEvent != nil {
		return mr.MonitorEvent.ExitEvent.ExitCode, nil
	} else if mr.ExitEvent != nil {
		return mr.ExitEvent.ExitCode, nil
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
	ID      string   `json:"id,omitempty"`
	Signal  string   `json:"signal,omitempty"`
	Timeout Duration `json:"timeout,omitempty"`
}

type RestartMachineInput struct {
	ID               string        `json:"id,omitempty"`
	Signal           string        `json:"signal,omitempty"`
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
	ID   string `json:"id,omitempty"`
	Kill bool   `json:"kill,omitempty"`
}

type MachineRestartPolicy string

var (
	MachineRestartPolicyNo        MachineRestartPolicy = "no"
	MachineRestartPolicyOnFailure MachineRestartPolicy = "on-failure"
	MachineRestartPolicyAlways    MachineRestartPolicy = "always"
)

type MachineRestart struct {
	Policy MachineRestartPolicy `json:"policy,omitempty"`
	// MaxRetries is only relevant with the on-failure policy.
	MaxRetries int `json:"max_retries,omitempty"`
}

type MachineMount struct {
	Encrypted bool   `json:"encrypted,omitempty"`
	Path      string `json:"path,omitempty"`
	SizeGb    int    `json:"size_gb,omitempty"`
	Volume    string `json:"volume,omitempty"`
	Name      string `json:"name,omitempty"`
}

type MachineGuest struct {
	CPUKind  string `json:"cpu_kind,omitempty"`
	CPUs     int    `json:"cpus,omitempty"`
	MemoryMB int    `json:"memory_mb,omitempty"`

	KernelArgs []string `json:"kernel_args,omitempty"`
}

func (mg *MachineGuest) SetSize(size string) error {
	guest, ok := MachinePresets[size]
	if !ok {
		var machine_type string

		if strings.HasPrefix(size, "shared") {
			machine_type = "shared"
		} else if strings.HasPrefix(size, "performance") {
			machine_type = "performance"
		} else {
			return fmt.Errorf("invalid machine preset requested, '%s', expected to start with 'shared' or 'performance'", size)
		}

		validSizes := []string{}
		for size := range MachinePresets {
			if strings.HasPrefix(size, machine_type) {
				validSizes = append(validSizes, size)
			}
		}
		sort.Strings(validSizes)
		return fmt.Errorf("'%s' is an invalid machine size, choose one of: %v", size, validSizes)
	}

	mg.CPUs = guest.CPUs
	mg.CPUKind = guest.CPUKind
	mg.MemoryMB = guest.MemoryMB
	return nil
}

// ToSize converts Guest into VMSize on a best effort way
func (mg *MachineGuest) ToSize() string {
	if mg == nil {
		return ""
	}
	switch mg.CPUKind {
	case "shared":
		return fmt.Sprintf("shared-cpu-%dx", mg.CPUs)
	case "performance":
		return fmt.Sprintf("performance-%dx", mg.CPUs)
	default:
		return "unknown"
	}
}

const (
	MIN_MEMORY_MB_PER_SHARED_CPU = 256
	MIN_MEMORY_MB_PER_CPU        = 2048

	MAX_MEMORY_MB_PER_SHARED_CPU = 2048
	MAX_MEMORY_MB_PER_CPU        = 8192
)

// TODO - Determine if we want allocate max memory allocation, or minimum per # cpus.
var MachinePresets map[string]*MachineGuest = map[string]*MachineGuest{
	"shared-cpu-1x": {CPUKind: "shared", CPUs: 1, MemoryMB: 1 * MIN_MEMORY_MB_PER_SHARED_CPU},
	"shared-cpu-2x": {CPUKind: "shared", CPUs: 2, MemoryMB: 2 * MIN_MEMORY_MB_PER_SHARED_CPU},
	"shared-cpu-4x": {CPUKind: "shared", CPUs: 4, MemoryMB: 4 * MIN_MEMORY_MB_PER_SHARED_CPU},
	"shared-cpu-8x": {CPUKind: "shared", CPUs: 8, MemoryMB: 8 * MIN_MEMORY_MB_PER_SHARED_CPU},

	"performance-1x":  {CPUKind: "performance", CPUs: 1, MemoryMB: 1 * MIN_MEMORY_MB_PER_CPU},
	"performance-2x":  {CPUKind: "performance", CPUs: 2, MemoryMB: 2 * MIN_MEMORY_MB_PER_CPU},
	"performance-4x":  {CPUKind: "performance", CPUs: 4, MemoryMB: 4 * MIN_MEMORY_MB_PER_CPU},
	"performance-8x":  {CPUKind: "performance", CPUs: 8, MemoryMB: 8 * MIN_MEMORY_MB_PER_CPU},
	"performance-16x": {CPUKind: "performance", CPUs: 16, MemoryMB: 16 * MIN_MEMORY_MB_PER_CPU},
}

type MachineMetrics struct {
	Port int    `toml:"port" json:"port,omitempty"`
	Path string `toml:"path" json:"path,omitempty"`
}

type MachineCheck struct {
	Port              *int                `json:"port,omitempty"`
	Type              *string             `json:"type,omitempty"`
	Interval          *Duration           `json:"interval,omitempty"`
	Timeout           *Duration           `json:"timeout,omitempty"`
	GracePeriod       *Duration           `json:"grace_period,omitempty"`
	HTTPMethod        *string             `json:"method,omitempty"`
	HTTPPath          *string             `json:"path,omitempty"`
	HTTPProtocol      *string             `json:"protocol,omitempty"`
	HTTPSkipTLSVerify *bool               `json:"tls_skip_verify,omitempty"`
	HTTPHeaders       []MachineHTTPHeader `json:"headers,omitempty"`
}

type MachineHTTPHeader struct {
	Name   string   `json:"name,omitempty"`
	Values []string `json:"values,omitempty"`
}

type ConsulCheckStatus string

const (
	Critical ConsulCheckStatus = "critical"
	Warning  ConsulCheckStatus = "warning"
	Passing  ConsulCheckStatus = "passing"
)

type MachineCheckStatus struct {
	Name      string            `json:"name,omitempty"`
	Status    ConsulCheckStatus `json:"status,omitempty"`
	Output    string            `json:"output,omitempty"`
	UpdatedAt *time.Time        `json:"updated_at,omitempty"`
}

type MachinePort struct {
	Port              *int               `json:"port,omitempty" toml:"port,omitempty"`
	StartPort         *int               `json:"start_port,omitempty" toml:"start_port,omitempty"`
	EndPort           *int               `json:"end_port,omitempty" toml:"end_port,omitempty"`
	Handlers          []string           `json:"handlers,omitempty" toml:"handlers,omitempty"`
	ForceHTTPS        bool               `json:"force_https,omitempty" toml:"force_https,omitempty"`
	TLSOptions        *TLSOptions        `json:"tls_options,omitempty" toml:"tls_options,omitempty"`
	HTTPOptions       *HTTPOptions       `json:"http_options,omitempty" toml:"http_options,omitempty"`
	ProxyProtoOptions *ProxyProtoOptions `json:"proxy_proto_options,omitempty" toml:"proxy_proto_options,omitempty"`
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

type ProxyProtoOptions struct {
	Version string `json:"version,omitempty" toml:"version,omitempty"`
}

type TLSOptions struct {
	ALPN              []string `json:"alpn,omitempty" toml:"alpn,omitempty"`
	Versions          []string `json:"versions,omitempty" toml:"versions,omitempty"`
	DefaultSelfSigned *bool    `json:"default_self_signed,omitempty" toml:"default_self_signed,omitempty"`
}

type HTTPOptions struct {
	Compress *bool                `json:"compress,omitempty" toml:"compress,omitempty"`
	Response *HTTPResponseOptions `json:"response,omitempty" toml:"response,omitempty"`
}

type HTTPResponseOptions struct {
	Headers map[string]any `json:"headers,omitempty" toml:"headers,omitempty"`
}

type MachineService struct {
	Protocol                 string                     `json:"protocol,omitempty" toml:"protocol,omitempty"`
	InternalPort             int                        `json:"internal_port,omitempty" toml:"internal_port,omitempty"`
	Autostop                 *bool                      `json:"autostop,omitempty"`
	Autostart                *bool                      `json:"autostart,omitempty"`
	MinMachinesRunning       *int                       `json:"min_machines_running,omitempty"`
	Ports                    []MachinePort              `json:"ports,omitempty" toml:"ports,omitempty"`
	Checks                   []MachineCheck             `json:"checks,omitempty" toml:"checks,omitempty"`
	Concurrency              *MachineServiceConcurrency `json:"concurrency,omitempty" toml:"concurrency"`
	ForceInstanceKey         *string                    `json:"force_instance_key" toml:"force_instance_key"`
	ForceInstanceDescription *string                    `json:"force_instance_description,omitempty" toml:"force_instance_description"`
}

type MachineServiceConcurrency struct {
	Type      string `json:"type,omitempty" toml:"type,omitempty"`
	HardLimit int    `json:"hard_limit,omitempty" toml:"hard_limit,omitempty"`
	SoftLimit int    `json:"soft_limit,omitempty" toml:"soft_limit,omitempty"`
}

type MachineConfig struct {
	// Fields managed from fly.toml
	// If you add anything here, ensure appconfig.Config.ToMachine() is updated
	Env      map[string]string       `json:"env,omitempty"`
	Init     MachineInit             `json:"init,omitempty"`
	Metadata map[string]string       `json:"metadata,omitempty"`
	Mounts   []MachineMount          `json:"mounts,omitempty"`
	Services []MachineService        `json:"services,omitempty"`
	Metrics  *MachineMetrics         `json:"metrics,omitempty"`
	Checks   map[string]MachineCheck `json:"checks,omitempty"`
	Statics  []*Static               `json:"statics,omitempty"`

	// Set by fly deploy or fly machines commands
	Image string `json:"image,omitempty"`

	// The following fields can only be set or updated by `fly machines run|update` commands
	// "fly deploy" must preserve them, if you add anything here, ensure it is propagated on deploys
	Schedule    string           `json:"schedule,omitempty"`
	AutoDestroy bool             `json:"auto_destroy,omitempty"`
	Restart     MachineRestart   `json:"restart,omitempty"`
	Guest       *MachineGuest    `json:"guest,omitempty"`
	DNS         *DNSConfig       `json:"dns,omitempty"`
	Processes   []MachineProcess `json:"processes,omitempty"`

	// Standbys enable a machine to be a standby for another. In the event of a hardware failure,
	// the standby machine will be started.
	Standbys []string `json:"standbys,omitempty"`

	StopConfig *StopConfig `json:"stop_config,omitempty"`

	// Deprecated: use Guest instead
	VMSize string `json:"size,omitempty"`
	// Deprecated: use Service.Autostart instead
	DisableMachineAutostart *bool `json:"disable_machine_autostart,omitempty"`
}

func (c *MachineConfig) ProcessGroup() string {
	// backwards compatible process_group getter.
	// from poking around, "fly_process_group" used to be called "process_group"
	// and since it's a metadata value, it's like a screenshot.
	// so we have 3 scenarios
	// - machines with only 'process_group'
	// - machines with both 'process_group' and 'fly_process_group'
	// - machines with only 'fly_process_group'

	if c.Metadata == nil {
		return ""
	}

	flyProcessGroup := c.Metadata[MachineConfigMetadataKeyFlyProcessGroup]
	if flyProcessGroup != "" {
		return flyProcessGroup
	}

	return c.Metadata["process_group"]
}

type Static struct {
	GuestPath string `toml:"guest_path" json:"guest_path" validate:"required"`
	UrlPrefix string `toml:"url_prefix" json:"url_prefix" validate:"required"`
}

type MachineInit struct {
	Exec       []string `json:"exec,omitempty"`
	Entrypoint []string `json:"entrypoint,omitempty"`
	Cmd        []string `json:"cmd,omitempty"`
	Tty        bool     `json:"tty,omitempty"`
}

type DNSConfig struct {
	SkipRegistration bool `json:"skip_registration,omitempty"`
}

type StopConfig struct {
	Timeout *Duration `json:"timeout,omitempty"`
	Signal  *string   `json:"signal,omitempty"`
}

type MachineLease struct {
	Status  string            `json:"status,omitempty"`
	Data    *MachineLeaseData `json:"data,omitempty"`
	Message string            `json:"message,omitempty"`
	Code    string            `json:"code,omitempty"`
}

type MachineLeaseData struct {
	Nonce     string `json:"nonce,omitempty"`
	ExpiresAt int64  `json:"expires_at,omitempty"`
	Owner     string `json:"owner,omitempty"`
}

type MachineStartResponse struct {
	Message       string `json:"message,omitempty"`
	Status        string `json:"status,omitempty"`
	PreviousState string `json:"previous_state,omitempty"`
}

type LaunchMachineInput struct {
	Config                  *MachineConfig `json:"config,omitempty"`
	Region                  string         `json:"region,omitempty"`
	Name                    string         `json:"name,omitempty"`
	SkipLaunch              bool           `json:"skip_launch,omitempty"`
	SkipServiceRegistration bool           `json:"skip_service_registration,omitempty"`

	LeaseTTL int `json:"lease_ttl,omitempty"`

	// Client side only
	ID                  string `json:"-"`
	SkipHealthChecks    bool   `json:"-"`
	RequiresReplacement bool   `json:"-"`
}

type MachineProcess struct {
	ExecOverride       []string          `json:"exec,omitempty"`
	EntrypointOverride []string          `json:"entrypoint,omitempty"`
	CmdOverride        []string          `json:"cmd,omitempty"`
	UserOverride       string            `json:"user,omitempty"`
	ExtraEnv           map[string]string `json:"env,omitempty"`
}

type MachineExecRequest struct {
	Cmd     string `json:"cmd,omitempty"`
	Timeout int    `json:"timeout,omitempty"`
}

type MachineExecResponse struct {
	ExitCode int32  `json:"exit_code,omitempty"`
	StdOut   string `json:"stdout,omitempty"`
	StdErr   string `json:"stderr,omitempty"`
}

type MachinePsResponse []ProcessStat

type ProcessStat struct {
	Pid           int32          `json:"pid"`
	Stime         uint64         `json:"stime"`
	Rtime         uint64         `json:"rtime"`
	Command       string         `json:"command"`
	Directory     string         `json:"directory"`
	Cpu           uint64         `json:"cpu"`
	Rss           uint64         `json:"rss"`
	ListenSockets []ListenSocket `json:"listen_sockets"`
}

type ListenSocket struct {
	Proto   string `json:"proto"`
	Address string `json:"address"`
}
