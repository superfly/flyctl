package machine

import (
	"context"
	"fmt"

	"github.com/docker/go-units"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/helpers"
	"github.com/superfly/flyctl/internal/flag"
)

// applyCacheDiskFlags changes only explicitly requested fields. The server
// validates size limits and mount paths so those rules can evolve independently.
func applyCacheDiskFlags(ctx context.Context, config *fly.MachineConfig) error {
	sizeSet := flag.IsSpecified(ctx, "cache-disk-size")
	pathSet := flag.IsSpecified(ctx, "cache-disk-path")
	if !sizeSet && !pathSet {
		return nil
	}
	disk := fly.MachineCacheDrive{}
	if config.CacheDrive != nil {
		disk = *config.CacheDrive
	}
	if sizeSet {
		size, err := helpers.ParseSize(flag.GetString(ctx, "cache-disk-size"), units.RAMInBytes, units.MiB)
		if err != nil {
			return fmt.Errorf("invalid cache disk size: %w", err)
		}
		if size < 0 {
			return fmt.Errorf("--cache-disk-size must not be negative")
		}
		disk.SizeMB = uint64(size)
	}
	if pathSet {
		disk.Path = flag.GetString(ctx, "cache-disk-path")
	}
	if disk.SizeMB == 0 {
		if pathSet && disk.Path != "" {
			return fmt.Errorf("--cache-disk-path requires a positive cache disk size")
		}
		config.CacheDrive = nil
	} else {
		if (config.CacheDrive == nil || config.CacheDrive.SizeMB == 0) && !pathSet {
			return fmt.Errorf("--cache-disk-path is required when adding a cache disk; specify an absolute mount path")
		}
		config.CacheDrive = &disk
	}

	return nil
}
