package scale

import (
	"strconv"

	"github.com/samber/lo"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/buildinfo"
)

type defaultValues struct {
	image          string
	guest          *api.MachineGuest
	guestPerGroup  map[string]*api.MachineGuest
	volsize        int
	volsizeByName  map[string]int
	releaseId      string
	releaseVersion string
	appConfig      *appconfig.Config
}

func newDefaults(appConfig *appconfig.Config, latest api.Release, machines []*api.Machine, volumes []api.Volume) *defaultValues {
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
	}

	volsizeByName := lo.Associate(volumes, func(v api.Volume) (string, int) {
		return v.Name, v.SizeGb
	})

	return &defaultValues{
		image:          latest.ImageRef,
		guest:          guest,
		guestPerGroup:  guestPerGroup,
		volsize:        1,
		volsizeByName:  volsizeByName,
		releaseId:      latest.ID,
		releaseVersion: strconv.Itoa(latest.Version),
		appConfig:      appConfig,
	}
}

func (d *defaultValues) ToMachineConfig(groupName string) (*api.MachineConfig, error) {
	mc, err := d.appConfig.ToMachineConfig(groupName, nil)
	if err != nil {
		return nil, err
	}

	if guest, ok := d.guestPerGroup[groupName]; ok {
		mc.Guest = guest
	} else {
		mc.Guest = d.guest
	}

	for idx := range mc.Mounts {
		mount := &mc.Mounts[idx]
		size := d.volsizeByName[mount.Name]
		if size == 0 {
			size = 1
		}
		mount.SizeGb = size
		mount.Encrypted = true
	}

	mc.Image = d.image
	mc.Metadata[api.MachineConfigMetadataKeyFlyReleaseId] = d.releaseId
	mc.Metadata[api.MachineConfigMetadataKeyFlyReleaseVersion] = d.releaseVersion
	mc.Metadata[api.MachineConfigMetadataKeyFlyctlVersion] = buildinfo.ParsedVersion().String()

	return mc, nil
}
