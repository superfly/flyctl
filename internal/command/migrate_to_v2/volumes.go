package migrate_to_v2

import (
	"context"
	"fmt"

	"github.com/superfly/flyctl/api"
)

func (m *v2PlatformMigrator) validateVolumes(ctx context.Context) error {
	if m.isPostgres {
		return nil
	}
	if m.appConfig.Mounts != nil {
		// TODO(ali): Validate this according to AppsV2 rules
		// return fmt.Errorf("cannot migrate app %s with [mounts] configuration, yet; watch https://community.fly.io for announcements about volume support with migrations", m.appCompact.Name)
	}
	for _, a := range m.oldAllocs {
		if len(a.AttachedVolumes.Nodes) > 1 {
			return fmt.Errorf("cannot migrate app %s because alloc %s has multiple volume attached", m.appCompact.Name, a.IDShort)
		}
	}
	// TODO(ali): Prohibit migration for apps with process group-dependent volume configurations
	// return fmt.Errorf("cannot migrate app %s because apps v2 does not yet support process group-specific mounts; watch https://community.fly.io for announcements about support for this feature", m.appCompact.Name, a.IDShort)
	return nil
}

func (m *v2PlatformMigrator) migrateAppVolumes(ctx context.Context) error {

	for _, vol := range m.appFull.Volumes.Nodes {
		// TODO(ali): Should we migrate _all_ volumes, or just the ones used currently?

		newVol, err := m.apiClient.ForkVolume(ctx, api.ForkVolumeInput{
			AppID:        m.appFull.ID,
			VolumeID:     vol.ID,
			MachinesOnly: true,
		})
		if err != nil {
			return err
		}

		allocId := ""
		if alloc := vol.AttachedAllocation; alloc != nil {
			allocId = alloc.ID
		}
		path := m.nomadVolPath(&vol)
		if path == "" && allocId != "" {
			return fmt.Errorf("volume %s[%s] is mounted on alloc %s, but has no mountpoint", vol.Name, vol.ID, allocId)
		}
		m.createdVolumes = append(m.createdVolumes, &NewVolume{
			vol:             newVol,
			previousAllocId: allocId,
			mountPoint:      path,
		})
	}
	return nil
}

func (m *v2PlatformMigrator) nomadVolPath(v *api.Volume) string {
	if v.AttachedAllocation == nil {
		return ""
	}

	// TODO(ali): Do process group-specific volumes change the logic here?
	for _, mount := range m.appConfig.Volumes() {
		if mount.Source == v.Name {
			return mount.Destination
		}
	}
	return ""
}

func (m *v2PlatformMigrator) markVolumesAsReadOnly(ctx context.Context) error {
	m.recovery.nomadVolsReadOnly = true
	panic("stub")
	return nil
}

func (m *v2PlatformMigrator) rollbackVolumesReadOnly(ctx context.Context) error {
	panic("stub")
	return nil
}

func nomadVolNameToV2VolName(name string) string {
	return name + "_machines"
}
