package deploy

import (
	"strconv"

	"github.com/samber/lo"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/helpers"
	"github.com/superfly/flyctl/internal/machine"
	"github.com/superfly/flyctl/terminal"
)

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

func (md *machineDeployment) launchInputForLaunch(processGroup string, guest *api.MachineGuest) (*api.LaunchMachineInput, error) {
	// Ignore the error because by this point we already check the processGroup exists
	mConfig, err := md.appConfig.ToMachineConfig(processGroup)
	if err != nil {
		return nil, err
	}
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
	}, nil
}

func (md *machineDeployment) launchInputForUpdate(origMachineRaw *api.Machine) (*api.LaunchMachineInput, error) {
	mID := origMachineRaw.ID
	processGroup := origMachineRaw.Config.ProcessGroup()

	// Ignore the error because by this point we already check the processGroup exists
	mConfig, err := md.appConfig.ToMachineConfig(processGroup)
	if err != nil {
		return nil, err
	}
	md.setMachineReleaseData(mConfig)
	// Keep fields that can't be controlled from fly.toml
	mConfig.Schedule = origMachineRaw.Config.Schedule
	mConfig.AutoDestroy = origMachineRaw.Config.AutoDestroy
	mConfig.Restart = helpers.Clone(origMachineRaw.Config.Restart)
	mConfig.Guest = helpers.Clone(origMachineRaw.Config.Guest)
	mConfig.DNS = helpers.Clone(origMachineRaw.Config.DNS)
	mConfig.FlyProxy = helpers.Clone(origMachineRaw.Config.FlyProxy)
	if len(origMachineRaw.Config.Processes) > 0 {
		mConfig.Processes = helpers.Clone(origMachineRaw.Config.Processes)
	}

	// Keep existing metadata not overridden by fresh machine config
	for k, v := range origMachineRaw.Config.Metadata {
		if _, exists := mConfig.Metadata[k]; !exists && !isFlyAppsPlatformMetadata(k) {
			mConfig.Metadata[k] = v
		}
	}

	// Mounts needs special treatment:
	//   * Volumes attached to existings machines can't be swapped by other volumes
	//   * The only allowed in-place operation is to update its destination mount path
	//   * The other option is to force a machine replacement to remove or attach a different volume
	mMounts := mConfig.Mounts
	oMounts := origMachineRaw.Config.Mounts
	if len(oMounts) != 0 {
		switch {
		case len(mMounts) == 0:
			// The mounts section was removed from fly.toml
			mID = "" // Forces machine replacement
			terminal.Warnf("Machine %s has a volume attached but fly.toml doesn't have a [mounts] section\n", mID)
		case oMounts[0].Name == "":
			// It's rare but can happen, we don't know the mounted volume name
			// so can't be sure it matches the mounts defined in fly.toml, in this
			// case assume we want to retain existing mount
			mMounts[0] = oMounts[0]
		case mMounts[0].Name != oMounts[0].Name:
			// The expected volume name for the machine and fly.toml are out sync
			// As we can't change the volume for a running machine, the only
			// way is to destroy the current machine and launch a new one with the new volume attached
			terminal.Warnf("Machine %s has volume '%s' attached but fly.toml have a different name: '%s'\n", mID, oMounts[0].Name, mMounts[0].Name)
			mMounts[0].Volume = md.volumes[0].ID
			mID = "" // Forces machine replacement
		case mMounts[0].Path != oMounts[0].Path:
			// The volume is the same but its mount path changed. Not a big deal.
			terminal.Warnf(
				"Updating the mount path for volume %s on machine %s from %s to %s due to fly.toml [mounts] destination value\n",
				oMounts[0].Volume, mID, oMounts[0].Path, mMounts[0].Path,
			)
			// Copy the volume id over because path is already correct
			mMounts[0].Volume = oMounts[0].Volume
		default:
			// In any other case retain the existing machine mounts
			mMounts[0] = oMounts[0]
		}
	} else if len(mMounts) != 0 {
		// Replace the machine because [mounts] section was added to fly.toml
		// and it is not possible to attach a volume to an existing machine.
		// The volume could be in a different zone than the machine.
		mID = "" // Forces machine replacement
		mMounts[0].Volume = md.volumes[0].ID
	}

	return &api.LaunchMachineInput{
		ID:      mID,
		AppID:   md.app.Name,
		OrgSlug: md.app.Organization.ID,
		Region:  origMachineRaw.Region,
		Config:  mConfig,
	}, nil
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
