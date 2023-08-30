package scale

import (
	"strconv"

	"github.com/samber/lo"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/buildinfo"
	"github.com/superfly/flyctl/internal/command/deploy"
)

type defaultValues struct {
	image           string
	guest           *api.MachineGuest
	guestPerGroup   map[string]*api.MachineGuest
	volsize         int
	volsizeByName   map[string]int
	releaseId       string
	releaseVersion  string
	appConfig       *appconfig.Config
	existingVolumes map[string]map[string][]*api.Volume
	snapshotID      *string
}

func newDefaults(appConfig *appconfig.Config, latest api.Release, machines []*api.Machine, volumes []api.Volume, snapshotID string, withNewVolumes bool) *defaultValues {
	guestPerGroup := lo.Associate(
		lo.Filter(machines, func(m *api.Machine, _ int) bool {
			return m.Config.Guest != nil
		}),
		func(m *api.Machine) (string, *api.MachineGuest) {
			return m.ProcessGroup(), m.Config.Guest
		},
	)

	// In case we haven't found a guest for the default,
	// scan all the existing groups and pick the first
	guest := guestPerGroup[appConfig.DefaultProcessName()]
	if guest == nil {
		for _, name := range appConfig.ProcessNames() {
			if v, ok := guestPerGroup[name]; ok {
				guest = v
				break
			}
		}

		// If we still don't have a guest size, just set it to the default one
		guest = new(api.MachineGuest)
		guest.SetSize(deploy.DefaultVMSize)
	}

	defaults := defaultValues{
		image:          latest.ImageRef,
		guest:          guest,
		guestPerGroup:  guestPerGroup,
		volsize:        1,
		releaseId:      latest.ID,
		releaseVersion: strconv.Itoa(latest.Version),
		appConfig:      appConfig,
	}

	if snapshotID != "" {
		defaults.snapshotID = &snapshotID
	}

	defaults.volsizeByName = lo.Reduce(volumes, func(agg map[string]int, v api.Volume, _ int) map[string]int {
		agg[v.Name] = lo.Max([]int{agg[v.Name], v.SizeGb})
		return agg
	}, make(map[string]int))

	if !withNewVolumes {
		defaults.existingVolumes = lo.MapValues(
			lo.GroupBy(
				lo.FilterMap(volumes, func(v api.Volume, _ int) (*api.Volume, bool) {
					return &v, !v.IsAttached()
				}),
				func(v *api.Volume) string { return v.Name },
			),
			func(vl []*api.Volume, _ string) map[string][]*api.Volume {
				return lo.GroupBy(vl, func(v *api.Volume) string {
					return v.Region
				})
			},
		)
	}
	return &defaults
}

func (d *defaultValues) ToMachineConfig(groupName string) (*api.MachineConfig, error) {
	mc, err := d.appConfig.ToMachineConfig(groupName, nil)
	if err != nil {
		return nil, err
	}

	mc.Image = d.image
	mc.Guest = lo.ValueOr(d.guestPerGroup, groupName, d.guest)
	mc.Mounts = lo.Map(mc.Mounts, func(mount api.MachineMount, _ int) api.MachineMount {
		mount.SizeGb = lo.ValueOr(d.volsizeByName, mount.Name, d.volsize)
		mount.Encrypted = true
		return mount
	})
	mc.Metadata[api.MachineConfigMetadataKeyFlyReleaseId] = d.releaseId
	mc.Metadata[api.MachineConfigMetadataKeyFlyReleaseVersion] = d.releaseVersion
	mc.Metadata[api.MachineConfigMetadataKeyFlyctlVersion] = buildinfo.ParsedVersion().String()

	return mc, nil
}

func (d *defaultValues) PopAvailableVolumes(mConfig *api.MachineConfig, region string, delta int) []*api.Volume {
	if delta <= 0 || len(mConfig.Mounts) == 0 {
		return nil
	}
	name := mConfig.Mounts[0].Name
	regionVolumes := d.existingVolumes[name][region]
	availableVolumes := regionVolumes[0:lo.Min([]int{len(regionVolumes), delta})]
	if len(availableVolumes) > 0 {
		d.existingVolumes[name][region] = lo.Drop(regionVolumes, len(availableVolumes))
	}
	return availableVolumes
}

func (d *defaultValues) CreateVolumeRequest(mConfig *api.MachineConfig, region string, delta int) *api.CreateVolumeRequest {
	if len(mConfig.Mounts) == 0 || delta <= 0 {
		return nil
	}
	mount := mConfig.Mounts[0]
	return &api.CreateVolumeRequest{
		Name:              mount.Name,
		Region:            region,
		SizeGb:            &mount.SizeGb,
		Encrypted:         api.Pointer(mount.Encrypted),
		RequireUniqueZone: api.Pointer(false),
		SnapshotID:        d.snapshotID,
	}
}
