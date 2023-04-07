package scale

import (
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/internal/appconfig"
)

type defaultValues struct {
	image          string
	guest          *api.MachineGuest
	guestPerGroup  map[string]*api.MachineGuest
	releaseId      string
	releaseVersion string
	appConfig      *appconfig.Config
}

func newDefaults(appConfig *appconfig.Config, machines []*api.Machine) *defaultValues {
	var (
		defaultGroupName = appConfig.DefaultProcessName()
		guest            *api.MachineGuest
		image            string
		releaseId        string
		releaseVersion   string
		guestPerGroup    = make(map[string]*api.MachineGuest)
	)

	for _, m := range machines {
		if len(m.Config.Metadata) > 0 {
			if releaseId == "" {
				releaseId = m.Config.Metadata[api.MachineConfigMetadataKeyFlyReleaseId]
			}
			if releaseVersion == "" {
				releaseVersion = m.Config.Metadata[api.MachineConfigMetadataKeyFlyReleaseVersion]
			}
			if image == "" {
				image = m.Config.Image
			}
		}

		groupName := m.ProcessGroup()
		if _, ok := guestPerGroup[groupName]; ok {
			continue
		} else if m.Config.Guest != nil {
			guestPerGroup[groupName] = m.Config.Guest
			if groupName == defaultGroupName {
				guest = m.Config.Guest
			}
		}
	}

	// In case we haven't found a guest for the default,
	// scan all the existing groups and pick the first
	if guest == nil {
		for _, name := range appConfig.ProcessNames() {
			if v, ok := guestPerGroup[name]; ok {
				guest = v
				break
			}
		}
	}

	return &defaultValues{
		image:          image,
		guest:          guest,
		guestPerGroup:  guestPerGroup,
		releaseId:      releaseId,
		releaseVersion: releaseVersion,
		appConfig:      appConfig,
	}
}

func (d *defaultValues) ToMachineConfig(groupName string) (*api.MachineConfig, error) {
	mc, err := d.appConfig.ToMachineConfig(groupName)
	if err != nil {
		return nil, err
	}

	if guest, ok := d.guestPerGroup[groupName]; ok {
		mc.Guest = guest
	} else {
		mc.Guest = d.guest
	}

	mc.Image = d.image
	mc.Metadata[api.MachineConfigMetadataKeyFlyReleaseId] = d.releaseId
	mc.Metadata[api.MachineConfigMetadataKeyFlyReleaseVersion] = d.releaseVersion

	return mc, nil
}
