package scale

import (
	"strconv"

	"github.com/samber/lo"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/buildinfo"
)

type defaultValues struct {
	image           string
	guest           *fly.MachineGuest
	guestPerGroup   map[string]*fly.MachineGuest
	volsize         int
	volsizeByName   map[string]int
	releaseId       string
	releaseVersion  string
	appConfig       *appconfig.Config
	existingVolumes map[string]map[string][]*fly.Volume
	snapshotID      *string
}

func newDefaults(appConfig *appconfig.Config, latest fly.Release, machines []*fly.Machine, volumes []fly.Volume, snapshotID string, withNewVolumes bool, fallbackGuest *fly.MachineGuest) *defaultValues {
	guestPerGroup := lo.Associate(
		lo.Filter(machines, func(m *fly.Machine, _ int) bool {
			return m.Config != nil && m.Config.Guest != nil
		}),
		func(m *fly.Machine) (string, *fly.MachineGuest) {
			return m.ProcessGroup(), m.Config.Guest
		},
	)

	// In case we haven't found a guest for the default,
	// scan all the existing groups and pick the first
	guest := guestPerGroup[appConfig.DefaultProcessName()]
	if guest == nil {
		guest = fallbackGuest
		for _, name := range appConfig.ProcessNames() {
			if v, ok := guestPerGroup[name]; ok {
				guest = v
				break
			}
		}
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

	defaults.volsizeByName = lo.Reduce(volumes, func(agg map[string]int, v fly.Volume, _ int) map[string]int {
		agg[v.Name] = lo.Max([]int{agg[v.Name], v.SizeGb})
		return agg
	}, make(map[string]int))

	if !withNewVolumes {
		defaults.existingVolumes = lo.MapValues(
			lo.GroupBy(
				lo.FilterMap(volumes, func(v fly.Volume, _ int) (*fly.Volume, bool) {
					return &v, !v.IsAttached()
				}),
				func(v *fly.Volume) string { return v.Name },
			),
			func(vl []*fly.Volume, _ string) map[string][]*fly.Volume {
				return lo.GroupBy(vl, func(v *fly.Volume) string {
					return v.Region
				})
			},
		)
	}
	return &defaults
}

func (d *defaultValues) ToMachineConfig(groupName string) (*fly.MachineConfig, error) {
	mc, err := d.appConfig.ToMachineConfig(groupName, nil)
	if err != nil {
		return nil, err
	}
	// Respect Guest if set by fly.toml
	if mc.Guest == nil {
		mc.Guest = lo.ValueOr(d.guestPerGroup, groupName, d.guest)
	}

	mc.Image = d.image
	mc.Mounts = lo.Map(mc.Mounts, func(mount fly.MachineMount, _ int) fly.MachineMount {
		mount.SizeGb = lo.ValueOr(d.volsizeByName, mount.Name, d.volsize)
		mount.Encrypted = true
		return mount
	})
	mc.Metadata[fly.MachineConfigMetadataKeyFlyReleaseId] = d.releaseId
	mc.Metadata[fly.MachineConfigMetadataKeyFlyReleaseVersion] = d.releaseVersion
	mc.Metadata[fly.MachineConfigMetadataKeyFlyctlVersion] = buildinfo.Version().String()

	return mc, nil
}

func (d *defaultValues) PopAvailableVolumes(mConfig *fly.MachineConfig, region string, delta int) []*fly.Volume {
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

func (d *defaultValues) CreateVolumeRequest(mConfig *fly.MachineConfig, region string, delta int) *fly.CreateVolumeRequest {
	if len(mConfig.Mounts) == 0 || delta <= 0 {
		return nil
	}
	mount := mConfig.Mounts[0]
	return &fly.CreateVolumeRequest{
		Name:                mount.Name,
		Region:              region,
		SizeGb:              &mount.SizeGb,
		Encrypted:           fly.Pointer(mount.Encrypted),
		RequireUniqueZone:   fly.Pointer(false),
		SnapshotID:          d.snapshotID,
		ComputeRequirements: mConfig.Guest,
		ComputeImage:        mConfig.Image,
	}
}
