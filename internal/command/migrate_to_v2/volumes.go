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
	m.usesForkedVolumes = numMounts != 0

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

	for _, vol := range m.appFull.Volumes.Nodes {
		// TODO(ali): Should we migrate _all_ volumes, or just the ones used currently?

		newVol, err := m.apiClient.ForkVolume(ctx, api.ForkVolumeInput{
			AppID:          m.appFull.ID,
			SourceVolumeID: vol.ID,
			MachinesOnly:   true,
			Name:           nomadVolNameToV2VolName(vol.Name),
			LockID:         m.appLock,
		})
		if err != nil {
			return err
		}

		allocId := ""
		path := ""
		if alloc := vol.AttachedAllocation; alloc != nil {
			allocId = alloc.ID
			path = m.nomadVolPath(&vol, alloc.TaskName)
			if path == "" {
				return fmt.Errorf("volume %s[%s] is mounted on alloc %s, but has no mountpoint", vol.Name, vol.ID, allocId)
			}
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
