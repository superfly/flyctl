package scale

import (
	"context"
	"fmt"

	"github.com/docker/go-units"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/helpers"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/iostreams"
)

// warnVMConfigMismatch checks if the local fly.toml has a [[vm]] section that
// conflicts with the values just applied by fly scale vm/memory. If so, it prints
// a warning to stderr. All errors are silently ignored — this is advisory only.
func warnVMConfigMismatch(ctx context.Context, group, sizeName string, memoryMB int) {
	cfg := loadLocalConfig(ctx)
	if cfg == nil {
		return
	}

	// No [[vm]] section means fly deploy preserves machine-level settings.
	if len(cfg.Compute) == 0 {
		return
	}

	if group == "" {
		group = cfg.DefaultProcessName()
	}

	compute := cfg.ComputeForGroup(group)
	if compute == nil {
		return
	}

	tomlSize, tomlMemoryMB := resolveComputeSettings(compute)

	sizeMismatch := sizeName != "" && tomlSize != sizeName
	memoryMismatch := memoryMB > 0 && tomlMemoryMB > 0 && tomlMemoryMB != memoryMB

	if !sizeMismatch && !memoryMismatch {
		return
	}

	io := iostreams.FromContext(ctx)
	colorize := io.ColorScheme()

	// Build "Machines now" line from the values just applied.
	machineSize := sizeName
	if machineSize == "" {
		machineSize = tomlSize
	}
	machineMem := memoryMB
	if machineMem == 0 {
		machineMem = tomlMemoryMB
	}

	fmt.Fprintln(io.ErrOut)
	fmt.Fprintln(io.ErrOut, colorize.Yellow(fmt.Sprintf(
		"%s Your fly.toml has a [[vm]] section with different settings than what was just applied.",
		colorize.WarningIcon(),
	)))
	fmt.Fprintln(io.ErrOut, colorize.Yellow(fmt.Sprintf(
		"  Machines now: size=%s memory=%dMB", machineSize, machineMem,
	)))
	fmt.Fprintln(io.ErrOut, colorize.Yellow(fmt.Sprintf(
		"  fly.toml:     size=%s memory=%dMB", tomlSize, tomlMemoryMB,
	)))
	fmt.Fprintln(io.ErrOut, colorize.Yellow(
		"The next fly deploy will override these changes with the fly.toml values.",
	))
	fmt.Fprintln(io.ErrOut, colorize.Yellow(
		"Update the [[vm]] section in fly.toml to make your changes permanent.",
	))
}

// loadLocalConfig attempts to load the local fly.toml. Returns nil on any failure.
func loadLocalConfig(ctx context.Context) *appconfig.Config {
	configPath := flag.GetAppConfigFilePath(ctx)
	if configPath == "" {
		configPath = "."
	}

	exists, err := appconfig.ConfigFileExistsAtPath(configPath)
	if err != nil || !exists {
		return nil
	}

	resolvedPath, err := appconfig.ResolveConfigFileFromPath(configPath)
	if err != nil {
		return nil
	}

	cfg, err := appconfig.LoadConfig(resolvedPath)
	if err != nil {
		return nil
	}

	return cfg
}

// resolveComputeSettings extracts the effective size name and memory MB from a
// Compute entry, replicating the logic from appconfig.computeToGuest (which is
// unexported). Returns the canonical size name and memory in MB.
func resolveComputeSettings(compute *appconfig.Compute) (sizeName string, memoryMB int) {
	size := fly.DefaultVMSize
	switch {
	case compute.Size != "":
		size = compute.Size
	case compute.MachineGuest != nil && compute.MachineGuest.GPUKind != "":
		size = fly.DefaultGPUVMSize
	}

	guest := &fly.MachineGuest{}
	if err := guest.SetSize(size); err != nil {
		return size, 0
	}

	sizeName = guest.ToSize()
	memoryMB = guest.MemoryMB

	// Memory string override (e.g., memory = "512mb")
	if compute.Memory != "" {
		if mb, err := helpers.ParseSize(compute.Memory, units.RAMInBytes, units.MiB); err == nil && mb > 0 {
			memoryMB = mb
		}
	}

	// Inline MachineGuest.MemoryMB override
	if compute.MachineGuest != nil && compute.MachineGuest.MemoryMB > 0 {
		memoryMB = compute.MachineGuest.MemoryMB
	}

	return sizeName, memoryMB
}
