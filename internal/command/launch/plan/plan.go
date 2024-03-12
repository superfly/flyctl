package plan

import (
	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/version"
)

type LaunchPlan struct {
	AppName string `json:"name"`
	OrgSlug string `json:"org"`

	RegionCode       string `json:"region"`
	HighAvailability bool   `json:"ha"`

	// Deprecated: The UI currently returns this instead of Compute, but new development should use Compute.
	CPUKind string `json:"vm_cpukind,omitempty"`
	// Deprecated: The UI currently returns this instead of Compute, but new development should use Compute.
	CPUs int `json:"vm_cpus,omitempty"`
	// Deprecated: The UI currently returns this instead of Compute, but new development should use Compute.
	MemoryMB int `json:"vm_memory,omitempty"`
	// Deprecated: The UI currently returns this instead of Compute, but new development should use Compute.
	VmSize string `json:"vm_size,omitempty"`

	// In the future, we'll use this over CPUKind, CPUs, MemoryMB, and VmSize.
	// As of writing this, however, the UI does not return this field.
	Compute []*appconfig.Compute `json:"compute"`

	HttpServicePort int `json:"http_service_port,omitempty"`

	Postgres      PostgresPlan      `json:"postgres"`
	Redis         RedisPlan         `json:"redis"`
	Sentry        bool              `json:"sentry"`
	ObjectStorage ObjectStoragePlan `json:"object_storage"`

	// We don't want to send the actual credentials back and forth, so this
	// indicates to the UI whether the user specified shadow bucket credentials via the command line.
	ObjectStorageHasShadowBucketCredentials bool `json:"object_storage_has_shadow_bucket_credentials"`

	ScannerFamily string          `json:"scanner_family"`
	FlyctlVersion version.Version `json:"flyctl_version"`
}

// Guest returns the guest described by the *raw* guest fields in a Plan.
// When the UI starts returning Compute, this will be deprecated.
func (p *LaunchPlan) Guest() *fly.MachineGuest {
	// TODO(Allison): Determine whether we should use VmSize or CPUKind/CPUs
	guest := fly.MachineGuest{
		CPUs:    p.CPUs,
		CPUKind: p.CPUKind,
	}
	if false {
		guest.SetSize(p.VmSize)
	}
	guest.MemoryMB = p.MemoryMB
	return &guest
}
