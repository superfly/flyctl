package migrate_to_v2

import (
	"context"
	"fmt"

	"github.com/samber/lo"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/flag"
	"golang.org/x/exp/slices"
)

const (
	forkedVolSuffix = "_machines"
)

func (m *v2PlatformMigrator) validateVolumes(ctx context.Context) error {
	if m.isPostgres {
		return nil
	}
	numMounts := len(m.appConfig.Volumes())
	if numMounts > 0 && !flag.GetBool(ctx, "experimental-volume-migration") {
		return fmt.Errorf(`migration for apps with volumes is experimental, and gated behind the --experimental-volume-migration flag
please do not use this on important/production apps yet
for updates, watch https://community.fly.io for announcements about volume migrations becoming stable`)
	}
	m.usesForkedVolumes = numMounts != 0
	if numMounts > 1 {
		return fmt.Errorf("cannot migrate app %s because it uses multiple [[mounts]], which are not yet supported on Apps V2.\nwatch https://community.fly.io for announcements about multiple volume mounts for Apps V2", m.appFull.Name)
	}
	for _, a := range m.oldAllocs {
		if len(a.AttachedVolumes.Nodes) > 1 {
			return fmt.Errorf("cannot migrate app %s because alloc %s has multiple volume attached", m.appCompact.Name, a.IDShort)
		}
	}
	return nil
}

func (m *v2PlatformMigrator) migrateAppVolumes(ctx context.Context) error {
	// NOTE: appconfig.Volume doesn't support the rare "processes" key on mounts
	//       this is extremely rarely used, and seemingly undocumented,
	//       but because of this rollback stores a copy of the old appconfig now
	//       and deploys the original config if something goes wrong.
	//       That said, once an app gets moved to V2, that mapping gets wiped.
	// (not an issue now, because we don't even support multiple volumes on v2,
	//  but it's worth documenting here nonetheless)
	m.appConfig.SetMounts(lo.Map(m.appConfig.Volumes(), func(v appconfig.Mount, _ int) appconfig.Mount {
		v.Source = nomadVolNameToV2VolName(v.Source)
		return v
	}))

	for _, vol := range m.appFull.Volumes.Nodes {
		// TODO(ali): Should we migrate _all_ volumes, or just the ones used currently?

		newVol, err := m.apiClient.ForkVolume(ctx, api.ForkVolumeInput{
			AppID:          m.appFull.ID,
			SourceVolumeID: vol.ID,
			MachinesOnly:   true,
			// TODO(ali): Do we want to rename their volumes?
			//            Currently, this adds a `_machines` suffix to the names
			Name:   nomadVolNameToV2VolName(vol.Name),
			LockID: m.appLock,
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
		m.replacedVolumes[vol.Name]++
	}
	return nil
}

func (m *v2PlatformMigrator) nomadVolPath(v *api.Volume) string {
	if v.AttachedAllocation == nil {
		return ""
	}

	// The config has already been patched to use the v2 volume names,
	// so we have to account for that here
	name := nomadVolNameToV2VolName(v.Name)

	// TODO(ali): Process group-specific volumes likely change the logic here
	for _, mount := range m.appConfig.Volumes() {
		if mount.Source == name {
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
