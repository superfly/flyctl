package deploy

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/helpers"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/buildinfo"
	"github.com/superfly/flyctl/internal/logger"
	"github.com/superfly/flyctl/terminal"
)

func makeTerminalLoggerQuiet(tb testing.TB) {
	var originalLogger = terminal.DefaultLogger
	terminal.DefaultLogger = logger.New(os.Stdout, logger.Error, true)

	tb.Cleanup(func() {
		terminal.DefaultLogger = originalLogger
	})
}

func TestLaunchInputForUpdate(t *testing.T) {
	makeTerminalLoggerQuiet(t)

	t.Run("Basic", testLaunchInputForBasic)
	t.Run("HostStatusUnreachable", testLaunchInputForUpdateHostStatusUnreachable)
	t.Run("Mounts", testLaunchInputForOnMounts)
	t.Run("MountsAndAutoResize", testLaunchInputForOnMountsAndAutoResize)
	t.Run("UpdateKeepUnmanagedFields", testLaunchInputForUpdateKeepUnmanagedFields)
	t.Run("UpdateClearStandbysWithServices", testLaunchInputForUpdateClearStandbysWithServices)
	t.Run("LaunchFiles", testLaunchInputForLaunchFiles)
	t.Run("LaunchFiles", testLaunchInputForUpdateFiles)
}

// Test the basic flow of launching, restarting and updating a machine for default process group
func testLaunchInputForBasic(t *testing.T) {
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
	want := &fly.LaunchMachineInput{
		Region: "scl",
		Config: &fly.MachineConfig{
			Env: map[string]string{
				"PRIMARY_REGION":    "scl",
				"OTHER":             "value",
				"FLY_PROCESS_GROUP": "app",
			},
			Image: "super/balloon",
			Metadata: map[string]string{
				"fly_platform_version": "v2",
				"fly_process_group":    "app",
				"fly_release_id":       "release_id",
				"fly_release_version":  "3",
				"fly_flyctl_version":   buildinfo.Version().String(),
			},
		},
	}
	li, err := md.launchInputForLaunch("", nil, nil)
	require.NoError(t, err)
	assert.Equal(t, want, li)

	// Restart the machine
	// Restarting the machine should change only the release id and version
	md.appConfig.Env["NOT_SET_ON_RESTART_ONLY"] = "true"
	md.img = "super/globe"
	md.releaseId = "new_release_id"
	md.releaseVersion = 4

	origMachineRaw := &fly.Machine{
		ID:         "ab1234567890",
		Region:     li.Region,
		Config:     helpers.Clone(li.Config),
		HostStatus: fly.HostStatusOk,
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
	origMachineRaw = &fly.Machine{
		ID:         li.ID,
		Region:     li.Region,
		Config:     helpers.Clone(li.Config),
		HostStatus: fly.HostStatusOk,
	}
	want.Config.Image = "super/globe"
	want.Config.Env["NOT_SET_ON_RESTART_ONLY"] = "true"
	li, err = md.launchInputForUpdate(origMachineRaw)
	require.NoError(t, err)
	assert.Equal(t, want, li)
}

// Test machines on unreachable hosts
func testLaunchInputForUpdateHostStatusUnreachable(t *testing.T) {
	md, err := stabMachineDeployment(&appconfig.Config{
		AppName:       "my-cool-app",
		PrimaryRegion: "scl",
		Env: map[string]string{
			"OTHER": "value",
		},
	})
	assert.NoError(t, err)

	li, err := md.launchInputForUpdate(&fly.Machine{
		ID:     "ab1234567890",
		Region: "ord",
		Config: &fly.MachineConfig{
			Metadata: map[string]string{"fly_process_group": "app"},
		},
		HostStatus: fly.HostStatusUnreachable,
	})
	require.NoError(t, err)
	require.True(t, li.RequiresReplacement)
	require.Equal(t, li.Region, "ord")

	// Updating an unreachable machine with a volume attached must fail until we can move the volume to another host
	md.appConfig.Mounts = []appconfig.Mount{{Source: "data", Destination: "/data"}}
	_, err = md.launchInputForUpdate(&fly.Machine{
		ID: "ab1234567890",
		IncompleteConfig: &fly.MachineConfig{
			Mounts: []fly.MachineMount{{Volume: "vol_attached", Path: "/data", Name: "data"}},
		},
		HostStatus: fly.HostStatusUnreachable,
	})
	require.ErrorContains(t, err, "unreachable")

	// Changing the volume name returns a new machine with a different volume attached
	md.volumes = map[string][]fly.Volume{
		"data": {
			{ID: "vol_10001", Name: "data"},
		},
	}
	li, err = md.launchInputForUpdate(&fly.Machine{
		ID: "ab1234567890",
		IncompleteConfig: &fly.MachineConfig{
			Mounts: []fly.MachineMount{{Volume: "vol_attached", Path: "/data", Name: "replace-me-because-i-m-different-fly-toml"}},
		},
		HostStatus: fly.HostStatusUnreachable,
	})
	require.NoError(t, err)
	require.True(t, li.RequiresReplacement)
	require.Equal(t, li.Config.Mounts, []fly.MachineMount{{Path: "/data", Volume: "vol_10001", Name: "data"}})
}

// Test Mounts
func testLaunchInputForOnMounts(t *testing.T) {
	md, err := stabMachineDeployment(&appconfig.Config{
		Mounts: []appconfig.Mount{{Source: "data", Destination: "/data"}},
	})
	assert.NoError(t, err)
	md.volumes = map[string][]fly.Volume{
		"data": {
			{ID: "vol_10001", Name: "data"},
			{ID: "vol_10002", Name: "data"},
			{ID: "vol_10003", Name: "data"},
		},
	}

	// New machine must get a volume attached
	li, err := md.launchInputForLaunch("", nil, nil)
	require.NoError(t, err)
	require.NotEmpty(t, li.Config.Mounts)
	assert.Equal(t, fly.MachineMount{Volume: "vol_10001", Path: "/data", Name: "data"}, li.Config.Mounts[0])

	// The machine already has a volume that matches fly.toml [mounts] section
	li, err = md.launchInputForUpdate(&fly.Machine{
		ID: "ab1234567890",
		Config: &fly.MachineConfig{
			Mounts: []fly.MachineMount{{Volume: "vol_attached", Path: "/data", Name: "data"}},
		},
		HostStatus: fly.HostStatusOk,
	})
	require.NoError(t, err)
	require.NotEmpty(t, li.Config.Mounts)
	assert.Equal(t, "ab1234567890", li.ID)
	assert.False(t, li.RequiresReplacement)
	assert.Equal(t, fly.MachineMount{Volume: "vol_attached", Path: "/data", Name: "data"}, li.Config.Mounts[0])

	// Update a machine with volume attached on a different path
	li, err = md.launchInputForUpdate(&fly.Machine{
		ID: "ab1234567890",
		Config: &fly.MachineConfig{
			Mounts: []fly.MachineMount{{Volume: "vol_attached", Path: "/update-me", Name: "data"}},
		},
		HostStatus: fly.HostStatusOk,
	})
	require.NoError(t, err)
	require.NotEmpty(t, li.Config.Mounts)
	assert.Equal(t, "ab1234567890", li.ID)
	assert.False(t, li.RequiresReplacement)
	assert.Equal(t, fly.MachineMount{Volume: "vol_attached", Path: "/data", Name: "data"}, li.Config.Mounts[0])

	// Updating a machine with an existing unnamed mount must keep the original mount as much as possible
	li, err = md.launchInputForUpdate(&fly.Machine{
		ID: "ab1234567890",
		Config: &fly.MachineConfig{
			Mounts: []fly.MachineMount{{Volume: "vol_attached", Path: "/keep-me"}},
		},
		HostStatus: fly.HostStatusOk,
	})
	require.NoError(t, err)
	require.NotEmpty(t, li.Config.Mounts)
	assert.Equal(t, "ab1234567890", li.ID)
	assert.False(t, li.RequiresReplacement)
	assert.Equal(t, fly.MachineMount{Volume: "vol_attached", Path: "/keep-me"}, li.Config.Mounts[0])

	// Updating a machine whose volume name doesn't match fly.toml's mount section must replace the machine altogether
	li, err = md.launchInputForUpdate(&fly.Machine{
		ID: "ab1234567890",
		Config: &fly.MachineConfig{
			Mounts: []fly.MachineMount{{Volume: "vol_attached", Path: "/replace-me", Name: "replace-me"}},
		},
		HostStatus: fly.HostStatusOk,
	})
	require.NoError(t, err)
	require.NotEmpty(t, li.Config.Mounts)
	assert.Equal(t, "ab1234567890", li.ID)
	assert.True(t, li.RequiresReplacement)
	assert.Equal(t, fly.MachineMount{Volume: "vol_10002", Path: "/data", Name: "data"}, li.Config.Mounts[0])

	// Updating a machine with an attached volume should trigger a replacement if fly.toml doesn't define one.
	md.appConfig.Mounts = nil
	li, err = md.launchInputForUpdate(&fly.Machine{
		ID: "ab1234567890",
		Config: &fly.MachineConfig{
			Mounts: []fly.MachineMount{{Volume: "vol_attached", Path: "/replace-me", Name: "replace-me"}},
		},
		HostStatus: fly.HostStatusOk,
	})
	require.NoError(t, err)
	assert.Equal(t, "ab1234567890", li.ID)
	assert.True(t, li.RequiresReplacement)
	assert.Empty(t, li.Config.Mounts)

}

// test mounts with auto volume resize
func testLaunchInputForOnMountsAndAutoResize(t *testing.T) {
	md, err := stabMachineDeployment(&appconfig.Config{
		Mounts: []appconfig.Mount{{
			Source:                  "data",
			Destination:             "/data",
			AutoExtendSizeThreshold: 80,
			AutoExtendSizeIncrement: "3GB",
			AutoExtendSizeLimit:     "100GB",
		}},
	})
	assert.NoError(t, err)
	md.volumes = map[string][]fly.Volume{
		"data": {
			{ID: "vol_10001", Name: "data"},
			{ID: "vol_10002", Name: "data"},
			{ID: "vol_10003", Name: "data"},
		},
	}

	// New machine must get a volume attached
	li, err := md.launchInputForLaunch("", nil, nil)
	require.NoError(t, err)
	require.NotEmpty(t, li.Config.Mounts)
	assert.Equal(t, fly.MachineMount{
		Volume:                 "vol_10001",
		Path:                   "/data",
		Name:                   "data",
		ExtendThresholdPercent: 80,
		AddSizeGb:              3,
		SizeGbLimit:            100,
	}, li.Config.Mounts[0])

	// The machine already has a volume that matches fly.toml [mounts] section
	// confirm new extend configs will be added
	li, err = md.launchInputForUpdate(&fly.Machine{
		ID: "ab1234567890",
		Config: &fly.MachineConfig{
			Mounts: []fly.MachineMount{{
				Volume:                 "vol_attached",
				Path:                   "/data",
				Name:                   "data",
				ExtendThresholdPercent: 90,
				AddSizeGb:              5,
				SizeGbLimit:            200,
			}},
		},
		HostStatus: fly.HostStatusOk,
	})
	require.NoError(t, err)
	require.NotEmpty(t, li.Config.Mounts)
	assert.Equal(t, "ab1234567890", li.ID)
	assert.Equal(t, fly.MachineMount{
		Volume:                 "vol_attached",
		Path:                   "/data",
		Name:                   "data",
		ExtendThresholdPercent: 80,
		AddSizeGb:              3,
		SizeGbLimit:            100,
	}, li.Config.Mounts[0])

	// Update a machine with volume attached on a different path
	li, err = md.launchInputForUpdate(&fly.Machine{
		ID: "ab1234567890",
		Config: &fly.MachineConfig{
			Mounts: []fly.MachineMount{{
				Volume:                 "vol_attached",
				Path:                   "/update-me",
				Name:                   "data",
				ExtendThresholdPercent: 90,
				AddSizeGb:              5,
				SizeGbLimit:            200,
			}},
		},
		HostStatus: fly.HostStatusOk,
	})
	require.NoError(t, err)
	require.NotEmpty(t, li.Config.Mounts)
	assert.Equal(t, "ab1234567890", li.ID)
	assert.Equal(t, fly.MachineMount{
		Volume:                 "vol_attached",
		Path:                   "/data",
		Name:                   "data",
		ExtendThresholdPercent: 80,
		AddSizeGb:              3,
		SizeGbLimit:            100,
	}, li.Config.Mounts[0])

	// Updating a machine with an existing unnamed mount must keep the original mount as much as possible
	li, err = md.launchInputForUpdate(&fly.Machine{
		ID: "ab1234567890",
		Config: &fly.MachineConfig{
			Mounts: []fly.MachineMount{{
				Volume:                 "vol_attached",
				Path:                   "/keep-me",
				ExtendThresholdPercent: 90,
				AddSizeGb:              5,
				SizeGbLimit:            200,
			}},
		},
		HostStatus: fly.HostStatusOk,
	})
	require.NoError(t, err)
	require.NotEmpty(t, li.Config.Mounts)
	assert.Equal(t, "ab1234567890", li.ID)
	assert.Equal(t, fly.MachineMount{
		Volume:                 "vol_attached",
		Path:                   "/keep-me",
		ExtendThresholdPercent: 80,
		AddSizeGb:              3,
		SizeGbLimit:            100,
	}, li.Config.Mounts[0])

	// Updating a machine whose volume name doesn't match fly.toml's mount section must replace the machine altogether
	li, err = md.launchInputForUpdate(&fly.Machine{
		ID: "ab1234567890",
		Config: &fly.MachineConfig{
			Mounts: []fly.MachineMount{{Volume: "vol_attached", Path: "/replace-me", Name: "replace-me"}},
		},
		HostStatus: fly.HostStatusOk,
	})
	require.NoError(t, err)
	require.NotEmpty(t, li.Config.Mounts)
	assert.Equal(t, "ab1234567890", li.ID)
	assert.True(t, li.RequiresReplacement)
	assert.Equal(t, fly.MachineMount{
		Volume:                 "vol_10002",
		Path:                   "/data",
		Name:                   "data",
		ExtendThresholdPercent: 80,
		AddSizeGb:              3,
		SizeGbLimit:            100,
	}, li.Config.Mounts[0])

	// Updating a machine with an attached volume should trigger a replacement if fly.toml doesn't define one.
	md.appConfig.Mounts = nil
	li, err = md.launchInputForUpdate(&fly.Machine{
		ID: "ab1234567890",
		Config: &fly.MachineConfig{
			Mounts: []fly.MachineMount{{
				Volume:                 "vol_attached",
				Path:                   "/replace-me",
				Name:                   "replace-me",
				ExtendThresholdPercent: 90,
				AddSizeGb:              5,
				SizeGbLimit:            200,
			}},
		},
		HostStatus: fly.HostStatusOk,
	})
	require.NoError(t, err)
	assert.Equal(t, "ab1234567890", li.ID)
	assert.True(t, li.RequiresReplacement)
	assert.Empty(t, li.Config.Mounts)
}

// Test restart or updating a machine propagates fields not under fly.toml control
func testLaunchInputForUpdateKeepUnmanagedFields(t *testing.T) {
	md, err := stabMachineDeployment(&appconfig.Config{
		AppName:       "my-cool-app",
		PrimaryRegion: "scl",
	})
	require.NoError(t, err)
	md.releaseId = "release_id"
	md.releaseVersion = 3

	origMachineRaw := &fly.Machine{
		ID:     "ab1234567890",
		Region: "ord",
		Config: &fly.MachineConfig{
			Schedule:    "24/7",
			AutoDestroy: true,
			Guest: &fly.MachineGuest{
				CPUKind: "other",
			},
			DNS: &fly.DNSConfig{
				SkipRegistration: true,
			},
			Processes: []fly.MachineProcess{{
				CmdOverride: []string{"foo"},
			}},
		},
		HostStatus: fly.HostStatusOk,
	}
	li, err := md.launchInputForUpdate(origMachineRaw)
	require.NoError(t, err)
	assert.Equal(t, "ab1234567890", li.ID)
	assert.Equal(t, "ord", li.Region)
	assert.Equal(t, "24/7", li.Config.Schedule)
	assert.Equal(t, true, li.Config.AutoDestroy)
	assert.Equal(t, &fly.MachineGuest{CPUKind: "other"}, li.Config.Guest)
	assert.Equal(t, &fly.DNSConfig{SkipRegistration: true}, li.Config.DNS)
	assert.Equal(t, []fly.MachineProcess{{CmdOverride: []string{"foo"}}}, li.Config.Processes)

	li = md.launchInputForRestart(origMachineRaw)
	assert.Equal(t, "ab1234567890", li.ID)
	assert.Equal(t, "ord", li.Region)
	assert.Equal(t, "24/7", li.Config.Schedule)
	assert.Equal(t, true, li.Config.AutoDestroy)
	assert.Equal(t, &fly.MachineGuest{CPUKind: "other"}, li.Config.Guest)
	assert.Equal(t, &fly.DNSConfig{SkipRegistration: true}, li.Config.DNS)
	assert.Equal(t, []fly.MachineProcess{{CmdOverride: []string{"foo"}}}, li.Config.Processes)
}

// Check that standby machines with services have their standbys list
// cleared.
func testLaunchInputForUpdateClearStandbysWithServices(t *testing.T) {
	md, err := stabMachineDeployment(&appconfig.Config{
		AppName:       "my-cool-app",
		PrimaryRegion: "scl",
		HTTPService: &appconfig.HTTPService{
			InternalPort: 8080,
		},
	})
	require.NoError(t, err)

	li, err := md.launchInputForUpdate(&fly.Machine{
		ID:     "ab1234567890",
		Region: "scl",
		Config: &fly.MachineConfig{
			Standbys: []string{"xy0987654321"},
		},
		HostStatus: fly.HostStatusOk,
	})
	require.NoError(t, err)

	assert.Equal(t, 0, len(li.Config.Standbys))
}

func testLaunchInputForLaunchFiles(t *testing.T) {
	md, err := stabMachineDeployment(&appconfig.Config{
		AppName:       "my-files-app",
		PrimaryRegion: "atl",
		MergedFiles: []*fly.File{
			{
				GuestPath: "/path/to/hello.txt",
				RawValue:  fly.StringPointer("aGVsbG8gd29ybGQK"),
			},
		},
	})
	require.NoError(t, err)
	md.releaseId = "release_id"
	md.releaseVersion = 3

	// Launch a new machine
	want := &fly.LaunchMachineInput{
		Region: "atl",
		Config: &fly.MachineConfig{
			Env: map[string]string{
				"PRIMARY_REGION":    "atl",
				"FLY_PROCESS_GROUP": "app",
			},
			Image: "super/balloon",
			Metadata: map[string]string{
				"fly_platform_version": "v2",
				"fly_process_group":    "app",
				"fly_release_id":       "release_id",
				"fly_release_version":  "3",
				"fly_flyctl_version":   buildinfo.Version().String(),
			},
			Files: []*fly.File{
				{
					GuestPath: "/path/to/hello.txt",
					RawValue:  fly.StringPointer("aGVsbG8gd29ybGQK"),
				},
			},
		},
	}
	li, err := md.launchInputForLaunch("", nil, nil)
	require.NoError(t, err)
	assert.Equal(t, want, li)
}

func testLaunchInputForUpdateFiles(t *testing.T) {
	md, err := stabMachineDeployment(&appconfig.Config{
		AppName:       "my-files-app",
		PrimaryRegion: "atl",
		MergedFiles: []*fly.File{
			{
				GuestPath:  "/path/to/config/yaml",
				SecretName: fly.StringPointer("SECRET_CONFIG"),
			},
			{
				GuestPath: "/path/to/hello.txt",
				RawValue:  fly.StringPointer("Z29vZGJ5ZQo="),
			},
		},
	})
	require.NoError(t, err)

	li, err := md.launchInputForUpdate(&fly.Machine{
		HostStatus: fly.HostStatusOk,
		Config: &fly.MachineConfig{
			Files: []*fly.File{
				{
					GuestPath: "/path/to/hello.txt",
					RawValue:  fly.StringPointer("aGVsbG8gd29ybGQK"),
				},
				{
					GuestPath: "/path/to/be/deleted",
					RawValue:  fly.StringPointer("ZGVsZXRlIG1lCg=="),
				},
			},
		},
	})
	require.NoError(t, err)
	assert.Equal(t, "/path/to/config/yaml", li.Config.Files[0].GuestPath)
	assert.Equal(t, "SECRET_CONFIG", *li.Config.Files[0].SecretName)
	assert.Equal(t, "/path/to/hello.txt", li.Config.Files[1].GuestPath)
	assert.Equal(t, "Z29vZGJ5ZQo=", *li.Config.Files[1].RawValue)
}
