package deploy

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/helpers"
	"github.com/superfly/flyctl/internal/appconfig"
)

// Test the basic flow of launching, restarting and updating a machine for default process group
func Test_launchInputFor_Basic(t *testing.T) {
	md, err := stabMachineDeployment(&appconfig.Config{
		AppName:       "my-cool-app",
		PrimaryRegion: "scl",
		Env: map[string]string{
			"OTHER": "value",
		},
	})
	require.NoError(t, err)
	md.releaseId = "release_id"
	md.releaseVersion = 3

	// Launch a new machine
	want := &api.LaunchMachineInput{
		OrgSlug: "my-dangling-org",
		Region:  "scl",
		Config: &api.MachineConfig{
			Env: map[string]string{
				"PRIMARY_REGION": "scl",
				"OTHER":          "value",
			},
			Image: "super/balloon",
			Metadata: map[string]string{
				"fly_platform_version": "v2",
				"fly_process_group":    "app",
				"fly_release_id":       "release_id",
				"fly_release_version":  "3",
			},
		},
	}
	li, err := md.launchInputForLaunch("", nil)
	require.NoError(t, err)
	assert.Equal(t, want, li)

	// Restart the machine
	// Restarting the machine should change only the release id and version
	md.appConfig.Env["NOT_SET_ON_RESTART_ONLY"] = "true"
	md.img = "super/globe"
	md.releaseId = "new_release_id"
	md.releaseVersion = 4

	origMachineRaw := &api.Machine{
		ID:     "ab1234567890",
		Region: li.Region,
		Config: helpers.Clone(li.Config),
	}
	// also must preserve any user's added metadata except for known fly metadata keys
	origMachineRaw.Config.Metadata["user-added-me"] = "keep it"
	origMachineRaw.Config.Metadata["fly-managed-postgres"] = "removes me"

	want.ID = origMachineRaw.ID
	want.Config.Metadata["fly_release_id"] = "new_release_id"
	want.Config.Metadata["fly_release_version"] = "4"
	want.Config.Metadata["user-added-me"] = "keep it"
	li = md.launchInputForRestart(origMachineRaw)
	assert.Equal(t, want, li)

	// Now updating the machines must include changes to appConfig
	origMachineRaw = &api.Machine{
		ID:     li.ID,
		Region: li.Region,
		Config: helpers.Clone(li.Config),
	}
	want.Config.Image = "super/globe"
	want.Config.Env["NOT_SET_ON_RESTART_ONLY"] = "true"
	li, err = md.launchInputForUpdate(origMachineRaw)
	require.NoError(t, err)
	assert.Equal(t, want, li)
}

// Test Mounts
func Test_launchInputFor_onMounts(t *testing.T) {
	md, err := stabMachineDeployment(&appconfig.Config{
		Mounts: []appconfig.Mount{{Source: "data", Destination: "/data"}},
	})
	assert.NoError(t, err)
	md.volumes = map[string][]api.Volume{
		"data": {{ID: "vol_12345", Name: "data"}},
	}

	// New machine must get a volume attached
	li, err := md.launchInputForLaunch("", nil)
	require.NoError(t, err)
	require.NotEmpty(t, li.Config.Mounts)
	assert.Equal(t, api.MachineMount{Volume: "vol_12345", Path: "/data", Name: "data"}, li.Config.Mounts[0])

	// The machine already has a volume that matches fly.toml [mounts] section
	li, err = md.launchInputForUpdate(&api.Machine{
		ID: "ab1234567890",
		Config: &api.MachineConfig{
			Mounts: []api.MachineMount{{Volume: "vol_attached", Path: "/data", Name: "data"}},
		},
	})
	require.NoError(t, err)
	require.NotEmpty(t, li.Config.Mounts)
	assert.Equal(t, "ab1234567890", li.ID)
	assert.Equal(t, api.MachineMount{Volume: "vol_attached", Path: "/data", Name: "data"}, li.Config.Mounts[0])

	// Update a machine with volume attached on a different path
	li, err = md.launchInputForUpdate(&api.Machine{
		ID: "ab1234567890",
		Config: &api.MachineConfig{
			Mounts: []api.MachineMount{{Volume: "vol_attached", Path: "/update-me", Name: "data"}},
		},
	})
	require.NoError(t, err)
	require.NotEmpty(t, li.Config.Mounts)
	assert.Equal(t, "ab1234567890", li.ID)
	assert.Equal(t, api.MachineMount{Volume: "vol_attached", Path: "/data", Name: "data"}, li.Config.Mounts[0])

	// Updating a machine with an existing unnamed mount must keep the original mount as much as possible
	li, err = md.launchInputForUpdate(&api.Machine{
		ID: "ab1234567890",
		Config: &api.MachineConfig{
			Mounts: []api.MachineMount{{Volume: "vol_attached", Path: "/keep-me"}},
		},
	})
	require.NoError(t, err)
	require.NotEmpty(t, li.Config.Mounts)
	assert.Equal(t, "ab1234567890", li.ID)
	assert.Equal(t, api.MachineMount{Volume: "vol_attached", Path: "/keep-me"}, li.Config.Mounts[0])

	// Updating a machine whose volume name doesn't match fly.toml's mount section must replace the machine altogether
	li, err = md.launchInputForUpdate(&api.Machine{
		ID: "ab1234567890",
		Config: &api.MachineConfig{
			Mounts: []api.MachineMount{{Volume: "vol_attached", Path: "/replace-me", Name: "replace-me"}},
		},
	})
	require.NoError(t, err)
	require.NotEmpty(t, li.Config.Mounts)
	assert.Equal(t, "", li.ID)
	assert.Equal(t, api.MachineMount{Volume: "vol_12345", Path: "/data", Name: "data"}, li.Config.Mounts[0])

	// Updating a machine with an attached volume should trigger a replacement if fly.toml doesn't define one.
	md.appConfig.Mounts = nil
	li, err = md.launchInputForUpdate(&api.Machine{
		ID: "ab1234567890",
		Config: &api.MachineConfig{
			Mounts: []api.MachineMount{{Volume: "vol_attached", Path: "/replace-me", Name: "replace-me"}},
		},
	})
	require.NoError(t, err)
	assert.Equal(t, "", li.ID)
	assert.Empty(t, li.Config.Mounts)
}

// Test restart or updating a machine propagates fields not under fly.toml control
func Test_launchInputForUpdate_keepUnmanagedFields(t *testing.T) {
	md, err := stabMachineDeployment(&appconfig.Config{
		AppName:       "my-cool-app",
		PrimaryRegion: "scl",
	})
	require.NoError(t, err)
	md.releaseId = "release_id"
	md.releaseVersion = 3

	origMachineRaw := &api.Machine{
		ID:     "ab1234567890",
		Region: "ord",
		Config: &api.MachineConfig{
			Schedule:    "24/7",
			AutoDestroy: true,
			Restart: api.MachineRestart{
				Policy: api.MachineRestartPolicyNo,
			},
			Guest: &api.MachineGuest{
				CPUKind: "other",
			},
			DNS: &api.DNSConfig{
				SkipRegistration: true,
			},
			FlyProxy: &api.MachineFlyProxy{
				AutostartMachine: api.Pointer(true),
				AutostopMachine:  api.Pointer(true),
			},
			Processes: []api.MachineProcess{{
				CmdOverride: []string{"foo"},
			}},
		},
	}
	li, err := md.launchInputForUpdate(origMachineRaw)
	require.NoError(t, err)
	assert.Equal(t, "ab1234567890", li.ID)
	assert.Equal(t, "ord", li.Region)
	assert.Equal(t, "24/7", li.Config.Schedule)
	assert.Equal(t, true, li.Config.AutoDestroy)
	assert.Equal(t, api.MachineRestart{Policy: api.MachineRestartPolicyNo}, li.Config.Restart)
	assert.Equal(t, &api.MachineGuest{CPUKind: "other"}, li.Config.Guest)
	assert.Equal(t, &api.DNSConfig{SkipRegistration: true}, li.Config.DNS)
	assert.Equal(t, &api.MachineFlyProxy{AutostartMachine: api.Pointer(true), AutostopMachine: api.Pointer(true)}, li.Config.FlyProxy)
	assert.Equal(t, []api.MachineProcess{{CmdOverride: []string{"foo"}}}, li.Config.Processes)

	li = md.launchInputForRestart(origMachineRaw)
	assert.Equal(t, "ab1234567890", li.ID)
	assert.Equal(t, "ord", li.Region)
	assert.Equal(t, "24/7", li.Config.Schedule)
	assert.Equal(t, true, li.Config.AutoDestroy)
	assert.Equal(t, api.MachineRestart{Policy: api.MachineRestartPolicyNo}, li.Config.Restart)
	assert.Equal(t, &api.MachineGuest{CPUKind: "other"}, li.Config.Guest)
	assert.Equal(t, &api.DNSConfig{SkipRegistration: true}, li.Config.DNS)
	assert.Equal(t, &api.MachineFlyProxy{AutostartMachine: api.Pointer(true), AutostopMachine: api.Pointer(true)}, li.Config.FlyProxy)
	assert.Equal(t, []api.MachineProcess{{CmdOverride: []string{"foo"}}}, li.Config.Processes)
}
