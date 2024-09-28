package deploy

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/samber/lo"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/internal/buildinfo"
	"github.com/superfly/flyctl/internal/machine"
	"github.com/superfly/flyctl/terminal"
)

func (md *machineDeployment) launchInputForRestart(origMachineRaw *fly.Machine) *fly.LaunchMachineInput {
	mConfig := machine.CloneConfig(origMachineRaw.Config)
	md.setMachineReleaseData(mConfig)

	return &fly.LaunchMachineInput{
		ID:         origMachineRaw.ID,
		Config:     mConfig,
		Region:     origMachineRaw.Region,
		SkipLaunch: skipLaunch(origMachineRaw, mConfig),
	}
}

func (md *machineDeployment) launchInputForLaunch(processGroup string, guest *fly.MachineGuest, standbyFor []string) (*fly.LaunchMachineInput, error) {
	mConfig, err := md.appConfig.ToMachineConfig(processGroup, nil)
	if err != nil {
		return nil, err
	}

	// Obey the Guest if already set from [[compute]] section
	if mConfig.Guest == nil {
		mConfig.Guest = guest
	}

	mConfig.Image = md.img
	md.setMachineReleaseData(mConfig)
	// Get the final process group and prevent empty string
	processGroup = mConfig.ProcessGroup()
	region := md.appConfig.PrimaryRegion

	if len(mConfig.Mounts) > 0 {
		mount0 := &mConfig.Mounts[0]
		vol := md.popVolumeFor(mount0.Name, region)
		if vol == nil {
			return nil, fmt.Errorf("New machine in group '%s' needs an unattached volume named '%s' in region '%s'", processGroup, mount0.Name, region)
		}
		mount0.Volume = vol.ID
	}

	if len(standbyFor) > 0 {
		mConfig.Standbys = standbyFor
		mConfig.Env["FLY_STANDBY_FOR"] = strings.Join(standbyFor, ",")
	}

	if hdid := md.appConfig.HostDedicationID; hdid != "" {
		mConfig.Guest.HostDedicationID = hdid
	}

	return &fly.LaunchMachineInput{
		Region:     region,
		Config:     mConfig,
		SkipLaunch: skipLaunch(nil, mConfig),
	}, nil
}

func (md *machineDeployment) launchInputForUpdate(origMachineRaw *fly.Machine) (*fly.LaunchMachineInput, error) {
	mID := origMachineRaw.ID
	machineShouldBeReplaced := dedicatedHostIdMismatch(origMachineRaw, md.appConfig)

	oConfig := origMachineRaw.GetConfig()
	if origMachineRaw.HostStatus != fly.HostStatusOk {
		machineShouldBeReplaced = true
	}

	processGroup := oConfig.ProcessGroup()

	mConfig, err := md.appConfig.ToMachineConfig(processGroup, oConfig)
	if err != nil {
		return nil, err
	}
	mConfig.Image = md.img
	md.setMachineReleaseData(mConfig)
	// Get the final process group and prevent empty string
	processGroup = mConfig.ProcessGroup()

	// Mounts needs special treatment:
	//   * Volumes attached to existings machines can't be swapped by other volumes
	//   * The only allowed in-place operation is to update its destination mount path
	//   * The other option is to force a machine replacement to remove or attach a different volume
	mMounts := mConfig.Mounts
	oMounts := oConfig.Mounts

	if len(oMounts) != 0 {
		var latestExtendThresholdPercent, latestAddSizeGb, latestSizeGbLimit int
		if len(mMounts) > 0 {
			latestExtendThresholdPercent = mMounts[0].ExtendThresholdPercent
			latestAddSizeGb = mMounts[0].AddSizeGb
			latestSizeGbLimit = mMounts[0].SizeGbLimit
		}
		switch {
		case len(mMounts) == 0:
			// The mounts section was removed from fly.toml
			machineShouldBeReplaced = true
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
			mount0 := &mMounts[0]
			terminal.Warnf("Machine %s has volume '%s' attached but fly.toml have a different name: '%s'\n", mID, oMounts[0].Name, mount0.Name)
			vol := md.popVolumeFor(mount0.Name, origMachineRaw.Region)
			if vol == nil {
				return nil, fmt.Errorf("machine in group '%s' needs an unattached volume named '%s' in region '%s'", processGroup, mount0.Name, origMachineRaw.Region)
			}
			mount0.Volume = vol.ID
			machineShouldBeReplaced = true
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

		if len(mMounts) > 0 {
			mMounts[0].ExtendThresholdPercent = latestExtendThresholdPercent
			mMounts[0].AddSizeGb = latestAddSizeGb
			mMounts[0].SizeGbLimit = latestSizeGbLimit
		}
	} else if len(mMounts) != 0 {
		// Replace the machine because [mounts] section was added to fly.toml
		// and it is not possible to attach a volume to an existing machine.
		// The volume could be in a different zone than the machine.
		mount0 := &mMounts[0]
		vol := md.popVolumeFor(mount0.Name, origMachineRaw.Region)
		if vol == nil {
			return nil, fmt.Errorf("machine in group '%s' needs an unattached volume named '%s' in region '%s'", processGroup, mMounts[0].Name, origMachineRaw.Region)
		}
		mount0.Volume = vol.ID
		machineShouldBeReplaced = true
	}

	if origMachineRaw.HostStatus != fly.HostStatusOk && len(oMounts) > 0 && len(mMounts) > 0 && oMounts[0].Volume == mMounts[0].Volume {
		// We are attempting to replace an unreachable machine but reusing the volume id that ties it to the dead host
		// TODO: Link to recovery instructions to manually create a new volume, empty or from snapshot, an only then recreate the machine.
		return nil, fmt.Errorf(
			"machine '%s' requires manual intervention, it can't be automatically replaced because its volume '%s' is on an unreachable host",
			mID,
			oMounts[0].Volume,
		)
	}

	// If this is a standby machine that now has a service, then clear
	// the standbys list.
	if len(mConfig.Services) > 0 && len(mConfig.Standbys) > 0 {
		mConfig.Standbys = nil
		delete(mConfig.Env, "FLY_STANDBY_FOR")
	}

	if hdid := md.appConfig.HostDedicationID; hdid != "" && hdid != oConfig.Guest.HostDedicationID {
		if len(oMounts) > 0 && len(mMounts) > 0 {
			// Attempting to rellocate a machine with a volume attached to a different host
			return nil, fmt.Errorf("can't rellocate machine '%s' to dedication id '%s' because it has an attached volume."+
				" Retry after forking the volume with `fly volume fork --host-dedication-id %s %s`", mID, hdid, hdid, mMounts[0].Volume)
		}
		machineShouldBeReplaced = true
		// Set HostDedicationID here for the apps that doesn't have a [[compute]] section in fly.toml
		// but sets it as a top level directive.
		// This also works when top level HDID is different than [compute.host_dedication_id]
		// because a flatten config also overrides the top level directive
		mConfig.Guest.HostDedicationID = hdid
	}

	return &fly.LaunchMachineInput{
		ID:                  mID,
		Region:              origMachineRaw.Region,
		Config:              mConfig,
		SkipLaunch:          skipLaunch(origMachineRaw, mConfig),
		RequiresReplacement: machineShouldBeReplaced,
	}, nil
}

func (md *machineDeployment) setMachineReleaseData(mConfig *fly.MachineConfig) {
	mConfig.Metadata = lo.Assign(mConfig.Metadata, map[string]string{
		fly.MachineConfigMetadataKeyFlyReleaseId:      md.releaseId,
		fly.MachineConfigMetadataKeyFlyReleaseVersion: strconv.Itoa(md.releaseVersion),
		fly.MachineConfigMetadataKeyFlyctlVersion:     buildinfo.Version().String(),
	})

	// These defaults should come from appConfig.ToMachineConfig() and set on launch;
	// leave them here for the moment becase very old machines may not have them
	// and we want to set in case of simple app restarts
	if _, ok := mConfig.Metadata[fly.MachineConfigMetadataKeyFlyPlatformVersion]; !ok {
		mConfig.Metadata[fly.MachineConfigMetadataKeyFlyPlatformVersion] = fly.MachineFlyPlatformVersion2
	}
	if _, ok := mConfig.Metadata[fly.MachineConfigMetadataKeyFlyProcessGroup]; !ok {
		mConfig.Metadata[fly.MachineConfigMetadataKeyFlyProcessGroup] = fly.MachineProcessGroupApp
	}

	// FIXME: Move this as extra metadata read from a machineDeployment argument
	// It is not clear we have to cleanup the postgres metadata
	if md.app.IsPostgresApp() {
		mConfig.Metadata[fly.MachineConfigMetadataKeyFlyManagedPostgres] = "true"
	} else {
		delete(mConfig.Metadata, fly.MachineConfigMetadataKeyFlyManagedPostgres)
	}
}

// Skip launching currently-stopped or suspended machines if:
// * any services use autoscaling (autostop or autostart).
// * it is a standby machine
func skipLaunch(origMachineRaw *fly.Machine, mConfig *fly.MachineConfig) bool {
	state := "<not-set>"
	if origMachineRaw != nil {
		state = origMachineRaw.State
	}

	switch {
	case state == fly.MachineStateStarted:
		return false
	case len(mConfig.Standbys) > 0:
		return true
	case state == fly.MachineStateStopped, state == fly.MachineStateSuspended:
		for _, s := range mConfig.Services {
			if (s.Autostop != nil && *s.Autostop != fly.MachineAutostopOff) || (s.Autostart != nil && *s.Autostart) {
				return true
			}
		}
	}
	return false
}
