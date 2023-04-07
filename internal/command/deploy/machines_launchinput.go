package deploy

import (
	"strconv"

	"github.com/samber/lo"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/helpers"
	"github.com/superfly/flyctl/internal/machine"
	"github.com/superfly/flyctl/terminal"
)

func (md *machineDeployment) resolveUpdatedMachineConfig(origMachineRaw *api.Machine, forReleaseCommand bool) *api.LaunchMachineInput {
	if md.restartOnly {
		return md.launchInputForRestart(origMachineRaw)
	}
	if forReleaseCommand {
		return md.launchInputForReleaseCommand(origMachineRaw)
	}
	return md.launchInputForUpdateOrNew(origMachineRaw, "")
}

func (md *machineDeployment) launchInputForRestart(origMachineRaw *api.Machine) *api.LaunchMachineInput {
	if origMachineRaw == nil {
		return nil
	}
	Config := machine.CloneConfig(origMachineRaw.Config)
	Config.Metadata = md.computeMachineConfigMetadata(Config)

	return &api.LaunchMachineInput{
		ID:      origMachineRaw.ID,
		AppID:   md.app.Name,
		OrgSlug: md.app.Organization.ID,
		Config:  Config,
		Region:  origMachineRaw.Region,
	}
}

func (md *machineDeployment) launchInputForUpdateOrNew(origMachineRaw *api.Machine, processGroup string) *api.LaunchMachineInput {
	// If machine exists we have to respect its ID, Region and Process Group
	if origMachineRaw == nil {
		origMachineRaw = &api.Machine{
			Region: md.appConfig.PrimaryRegion,
			Config: &api.MachineConfig{},
		}
	} else if processGroup == "" {
		processGroup = origMachineRaw.Config.ProcessGroup()
	}

	// Ignore the error because by this point we already check the processGroup exists
	mConfig, _ := md.appConfig.ToMachineConfig(processGroup)
	mConfig.Guest = helpers.Clone(origMachineRaw.Config.Guest)
	md.setMachineReleaseData(mConfig)

	// Keep existing metadata not overridden by fresh machine config
	for k, v := range origMachineRaw.Config.Metadata {
		if _, exists := mConfig.Metadata[k]; !exists && !isFlyAppsPlatformMetadata(k) {
			mConfig.Metadata[k] = v
		}
	}

	// Mounts needs special treatment:
	//   * Volumes attached to existings machines can't be swapped by other volumes
	//   * The only possible operation is to update its destination mount path
	if len(origMachineRaw.Config.Mounts) != 0 {
		origMount := &(origMachineRaw.Config.Mounts[0])
		if len(mConfig.Mounts) == 0 {
			terminal.Warnf("Machine %s has a volume attached but fly.toml doesn't have a [mounts] section\n", origMachineRaw.ID)
		} else if cfgMount := &(mConfig.Mounts[0]); cfgMount.Path != origMount.Path {
			terminal.Warnf(
				"Updating the mount path for volume %s on machine %s from %s to %s due to fly.toml [mounts] destination value\n",
				origMount.Volume, origMachineRaw.ID, origMount.Path, cfgMount.Path,
			)
			origMount.Path = cfgMount.Path
		}
		mConfig.Mounts = origMachineRaw.Config.Mounts
	} else if len(mConfig.Mounts) != 0 {
		mConfig.Mounts[0].Volume = md.volumes[0].ID
	}

	return &api.LaunchMachineInput{
		ID:      origMachineRaw.ID,
		AppID:   md.app.Name,
		OrgSlug: md.app.Organization.ID,
		Region:  origMachineRaw.Region,
		Config:  mConfig,
	}
}

func (md *machineDeployment) defaultMachineMetadata() map[string]string {
	res := map[string]string{
		api.MachineConfigMetadataKeyFlyPlatformVersion: api.MachineFlyPlatformVersion2,
		api.MachineConfigMetadataKeyFlyReleaseId:       md.releaseId,
		api.MachineConfigMetadataKeyFlyReleaseVersion:  strconv.Itoa(md.releaseVersion),
		api.MachineConfigMetadataKeyFlyProcessGroup:    api.MachineProcessGroupApp,
	}
	if md.app.IsPostgresApp() {
		res[api.MachineConfigMetadataKeyFlyManagedPostgres] = "true"
	}
	return res
}

func isFlyAppsPlatformMetadata(key string) bool {
	return key == api.MachineConfigMetadataKeyFlyPlatformVersion ||
		key == api.MachineConfigMetadataKeyFlyReleaseId ||
		key == api.MachineConfigMetadataKeyFlyReleaseVersion ||
		key == api.MachineConfigMetadataKeyFlyManagedPostgres
}

func (md *machineDeployment) computeMachineConfigMetadata(config *api.MachineConfig) map[string]string {
	return lo.Assign(
		md.defaultMachineMetadata(),
		lo.OmitBy(config.Metadata, func(k, v string) bool {
			return isFlyAppsPlatformMetadata(k)
		}))
}

func (md *machineDeployment) setMachineReleaseData(mConfig *api.MachineConfig) {
	mConfig.Image = md.img
	mConfig.Metadata = md.computeMachineConfigMetadata(mConfig)
}
