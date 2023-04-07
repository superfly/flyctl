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
	switch {
	case md.restartOnly:
		return md.launchInputForRestart(origMachineRaw)
	case forReleaseCommand:
		return md.launchInputForReleaseCommand(origMachineRaw)
	case origMachineRaw == nil:
		return md.launchInputForLaunch("", nil)
	default:
		return md.launchInputForUpdate(origMachineRaw)
	}
}

func (md *machineDeployment) launchInputForRestart(origMachineRaw *api.Machine) *api.LaunchMachineInput {
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

func (md *machineDeployment) launchInputForLaunch(processGroup string, guest *api.MachineGuest) *api.LaunchMachineInput {
	// Ignore the error because by this point we already check the processGroup exists
	mConfig, _ := md.appConfig.ToMachineConfig(processGroup)
	mConfig.Guest = guest
	md.setMachineReleaseData(mConfig)

	if len(mConfig.Mounts) != 0 {
		mConfig.Mounts[0].Volume = md.volumes[0].ID
	}

	return &api.LaunchMachineInput{
		AppID:   md.app.Name,
		OrgSlug: md.app.Organization.ID,
		Region:  md.appConfig.PrimaryRegion,
		Config:  mConfig,
	}
}

func (md *machineDeployment) launchInputForUpdate(origMachineRaw *api.Machine) *api.LaunchMachineInput {
	mID := origMachineRaw.ID
	processGroup := origMachineRaw.Config.ProcessGroup()

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
	//   * The other option is to force a machine replacement to remove or attach a different volume
	mMounts := mConfig.Mounts
	oMounts := origMachineRaw.Config.Mounts
	if len(oMounts) != 0 {
		switch {
		case len(mMounts) == 0:
			terminal.Warnf("Machine %s has a volume attached but fly.toml doesn't have a [mounts] section\n", mID)
			mID = "" // Forces machine replacement
		case mMounts[0].Name != oMounts[0].Name && oMounts[0].Name != "":
			terminal.Warnf("Machine %s has volume '%s' attached but fly.toml have a different name: '%s'\n", mID, oMounts[0].Name, mMounts[0].Name)
			mMounts[0].Volume = md.volumes[0].ID
			mID = "" // Forces machine replacement
		case mMounts[0].Path != oMounts[0].Path:
			terminal.Warnf(
				"Updating the mount path for volume %s on machine %s from %s to %s due to fly.toml [mounts] destination value\n",
				oMounts[0].Volume, mID, oMounts[0].Path, mMounts[0].Path,
			)
			mMounts[0].Volume = oMounts[0].Volume
		default:
			mMounts[0] = oMounts[0]
		}
	} else if len(mMounts) != 0 {
		mMounts[0].Volume = md.volumes[0].ID
		mID = "" // Forces machine replacement
	}

	return &api.LaunchMachineInput{
		ID:      mID,
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
