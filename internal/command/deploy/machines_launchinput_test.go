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
			Services: []api.MachineService{},
			Checks:   map[string]api.MachineCheck{},
		},
	}
	li := md.launchInputForLaunch("", nil)
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
	li = md.launchInputForUpdate(origMachineRaw)
	assert.Equal(t, want, li)
}

// Test Mounts
func Test_launchInputFor_onMounts(t *testing.T) {
	md, err := stabMachineDeployment(&appconfig.Config{
		Mounts: &appconfig.Volume{Source: "data", Destination: "/data"},
	})
	assert.NoError(t, err)
	md.volumes = []api.Volume{{ID: "vol_12345", Name: "data"}}

	// New machine must get a volume attached
	li := md.launchInputForLaunch("", nil)
	require.NotEmpty(t, li.Config.Mounts)
	assert.Equal(t, api.MachineMount{Volume: "vol_12345", Path: "/data", Name: "data"}, li.Config.Mounts[0])

	// The machine already has a volume that matches fly.toml [mounts] section
	li = md.launchInputForUpdate(&api.Machine{
		ID: "ab1234567890",
		Config: &api.MachineConfig{
			Mounts: []api.MachineMount{{Volume: "vol_attached", Path: "/data", Name: "data"}},
		},
	})
	require.NotEmpty(t, li.Config.Mounts)
	assert.Equal(t, "ab1234567890", li.ID)
	assert.Equal(t, api.MachineMount{Volume: "vol_attached", Path: "/data", Name: "data"}, li.Config.Mounts[0])

	// Update a machine with volume attached on a different path
	li = md.launchInputForUpdate(&api.Machine{
		ID: "ab1234567890",
		Config: &api.MachineConfig{
			Mounts: []api.MachineMount{{Volume: "vol_attached", Path: "/update-me", Name: "data"}},
		},
	})
	require.NotEmpty(t, li.Config.Mounts)
	assert.Equal(t, "ab1234567890", li.ID)
	assert.Equal(t, api.MachineMount{Volume: "vol_attached", Path: "/data", Name: "data"}, li.Config.Mounts[0])

	// Updating a machine with an existing unnamed mount must keep the original mount as much as possible
	li = md.launchInputForUpdate(&api.Machine{
		ID: "ab1234567890",
		Config: &api.MachineConfig{
			Mounts: []api.MachineMount{{Volume: "vol_attached", Path: "/keep-me"}},
		},
	})
	require.NotEmpty(t, li.Config.Mounts)
	assert.Equal(t, "ab1234567890", li.ID)
	assert.Equal(t, api.MachineMount{Volume: "vol_attached", Path: "/keep-me"}, li.Config.Mounts[0])

	// Updating a machine whose volume name doesn't match fly.toml's mount section must replace the machine altogether
	li = md.launchInputForUpdate(&api.Machine{
		ID: "ab1234567890",
		Config: &api.MachineConfig{
			Mounts: []api.MachineMount{{Volume: "vol_attached", Path: "/replace-me", Name: "replace-me"}},
		},
	})
	require.NotEmpty(t, li.Config.Mounts)
	assert.Equal(t, "", li.ID)
	assert.Equal(t, api.MachineMount{Volume: "vol_12345", Path: "/data", Name: "data"}, li.Config.Mounts[0])

	// Updating a machine with an attached volume should trigger a replacement if fly.toml doesn't define one.
	md.appConfig.Mounts = nil
	li = md.launchInputForUpdate(&api.Machine{
		ID: "ab1234567890",
		Config: &api.MachineConfig{
			Mounts: []api.MachineMount{{Volume: "vol_attached", Path: "/replace-me", Name: "replace-me"}},
		},
	})
	assert.Equal(t, "", li.ID)
	assert.Empty(t, li.Config.Mounts)
}
