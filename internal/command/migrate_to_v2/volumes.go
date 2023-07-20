package migrate_to_v2

import (
	"context"
	"fmt"
	"strings"

	"github.com/samber/lo"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/internal/appconfig"
	"golang.org/x/exp/slices"
)

const (
	forkedVolSuffix = "_machines"
)

func (m *v2PlatformMigrator) validateVolumes(ctx context.Context) error {
	if m.isPostgres {
		return nil
	}
	numMounts := len(m.appConfig.Mounts)
	m.usesForkedVolumes = numMounts != 0 && len(m.oldAttachedVolumes) > 0

	volsPerProcess := map[string]int{}
	for _, mount := range m.appConfig.Mounts {
		processes := mount.Processes
		if len(processes) == 0 {
			processes = m.appConfig.ProcessNames()
		}
		for _, p := range processes {
			volsPerProcess[p]++
		}
	}
	unmigratableProcesses := lo.FilterMap(lo.Keys(volsPerProcess), func(k string, _ int) (string, bool) {
		if volsPerProcess[k] > 1 {
			return fmt.Sprintf("%s (%d)", k, volsPerProcess[k]), true
		}
		return "", false
	})
	if len(unmigratableProcesses) != 0 {
		slices.Sort(unmigratableProcesses)
		processesInfo := ""
		if len(unmigratableProcesses) == 1 {
			processesInfo = fmt.Sprintf("process %s", unmigratableProcesses[0])
		} else {
			processesInfo = fmt.Sprintf("processes %s", strings.Join(unmigratableProcesses, ", "))
		}
		return fmt.Errorf("cannot migrate app %s because it uses multiple mounts for %s, which are not yet supported on Apps V2.\nwatch https://community.fly.io for announcements about multiple volume mounts for Apps V2", m.appFull.Name, processesInfo)
	}
	for _, a := range m.oldAllocs {
		if len(a.AttachedVolumes.Nodes) > 1 {
			return fmt.Errorf("cannot migrate app %s because alloc %s has multiple volume attached", m.appCompact.Name, a.IDShort)
		}
	}
	return nil
}

func (m *v2PlatformMigrator) migrateAppVolumes(ctx context.Context) error {
	m.appConfig.SetMounts(lo.Map(m.appConfig.Mounts, func(v appconfig.Mount, _ int) appconfig.Mount {
		v.Source = nomadVolNameToV2VolName(v.Source)
		if len(v.Processes) == 0 {
			v.Processes = m.appConfig.ProcessNames()
		}
		return v
	}))

	for _, vol := range m.oldAttachedVolumes {
		newVol, err := m.flapsClient.CreateVolume(ctx, api.CreateVolumeRequest{
			SourceVolumeID: &vol.ID,
			Name:           nomadVolNameToV2VolName(vol.Name),
			// LockID:         m.appLock, TODO LOCK
		})
		if err != nil && strings.HasSuffix(err.Error(), " is not a valid candidate") {
			return fmt.Errorf("unfortunately the worker hosting your volume %s (%s) does not have capacity for another volume to support the migration; some other options: 1) try again later and there might be more space on the worker, 2) run a manual migration https://community.fly.io/t/manual-migration-to-apps-v2/11870, or 3) wait until we support volume migrations across workers (we're working on it!)", vol.ID, vol.Name)
		} else if err != nil {
			return err
		}

		allocId := ""
		path := ""
		if allocId := vol.AttachedAllocation; allocId != nil {
			alloc, ok := lo.Find(m.oldAllocs, func(a *api.AllocationStatus) bool {
				return a.ID == *allocId
			})
			if !ok {
				return fmt.Errorf("volume %s[%s] is attached to alloc %s, but that alloc is not running", vol.Name, vol.ID, *allocId)
			}
			path = m.nomadVolPath(&vol, alloc.TaskName)
			if path == "" {
				return fmt.Errorf("volume %s[%s] is mounted on alloc %s, but has no mountpoint", vol.Name, vol.ID, *allocId)
			}
		}
		if m.verbose {
			fmt.Fprintf(m.io.Out, "Forked volume %s[%s] into %s[%s]\n", vol.Name, vol.ID, newVol.Name, newVol.ID)
		}
		m.createdVolumes = append(m.createdVolumes, &NewVolume{
			vol:             newVol,
			previousAllocId: allocId,
			mountPoint:      path,
		})
		m.replacedVolumes[vol.Name]++
	}
	return nil
}

func (m *v2PlatformMigrator) nomadVolPath(v *api.Volume, group string) string {
	if v.AttachedAllocation == nil {
		return ""
	}

	// The config has already been patched to use the v2 volume names,
	// so we have to account for that here
	name := nomadVolNameToV2VolName(v.Name)

	for _, mount := range m.appConfig.Mounts {
		if mount.Source == name && lo.Contains(mount.Processes, group) {
			return mount.Destination
		}
	}
	return ""
}

// Must run *after* allocs are filtered
func (m *v2PlatformMigrator) resolveOldVolumes() {
	m.oldAttachedVolumes = lo.Filter(m.appFull.Volumes.Nodes, func(v api.Volume, _ int) bool {
		if v.AttachedAllocation != nil {
			for _, a := range m.oldAllocs {
				if a.ID == *v.AttachedAllocation {
					return true
				}
			}
		}
		return false
	})
}

func (m *v2PlatformMigrator) printReplacedVolumes() {
	if len(m.replacedVolumes) == 0 {
		return
	}
	fmt.Fprintf(m.io.Out, "The following volumes have been migrated to new volumes, and are no longer needed:\n")
	keys := lo.Keys(m.replacedVolumes)
	slices.Sort(keys)
	for _, name := range keys {
		num := m.replacedVolumes[name]
		s := lo.Ternary(num == 1, "", "s")
		fmt.Fprintf(m.io.Out, " * %d volume%s named '%s' [replaced by '%s']\n", num, s, name, nomadVolNameToV2VolName(name))
	}
}

func nomadVolNameToV2VolName(name string) string {
	return name + forkedVolSuffix
}
