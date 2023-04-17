package deploy

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/machine"
)

func stabMachineDeployment(appConfig *appconfig.Config) (*machineDeployment, error) {
	md := &machineDeployment{
		app: &api.AppCompact{
			ID: "my-cool-app",
			Organization: &api.OrganizationBasic{
				ID: "my-dangling-org",
			},
		},
		img:        "super/balloon",
		appConfig:  appConfig,
		machineSet: machine.NewMachineSet(nil, nil, nil),
	}
	return md, nil
}

func Test_resolveUpdatedMachineConfig_Basic(t *testing.T) {
	md, err := stabMachineDeployment(&appconfig.Config{
		AppName: "my-cool-app",
		Env: map[string]string{
			"PRIMARY_REGION": "scl",
			"OTHER":          "value",
		},
	})
	require.NoError(t, err)
	li, err := md.launchInputForLaunch("", nil)
	require.NoError(t, err)
	assert.Equal(t, &api.LaunchMachineInput{
		OrgSlug: "my-dangling-org",
		Config: &api.MachineConfig{
			Env: map[string]string{
				"PRIMARY_REGION":    "scl",
				"OTHER":             "value",
				"FLY_PROCESS_GROUP": "app",
			},
			Image: "super/balloon",
			Metadata: map[string]string{
				"fly_platform_version": "v2",
				"fly_process_group":    "app",
				"fly_release_id":       "",
				"fly_release_version":  "0",
			},
		},
	}, li)
}

// Test any LaunchMachineInput field that must not be set on a machine
// used to run release command.
func Test_resolveUpdatedMachineConfig_ReleaseCommand(t *testing.T) {
	md, err := stabMachineDeployment(&appconfig.Config{
		AppName: "my-cool-app",
		Env: map[string]string{
			"PRIMARY_REGION": "scl",
			"OTHER":          "value",
		},
		Metrics: &api.MachineMetrics{
			Port: 9000,
			Path: "/prometheus",
		},
		Deploy: &appconfig.Deploy{
			ReleaseCommand: "touch sky",
		},
		Mounts: []appconfig.Mount{{
			Source:      "data",
			Destination: "/data",
		}},
		Checks: map[string]*appconfig.ToplevelCheck{
			"alive": {
				Port: api.Pointer(8080),
				Type: api.Pointer("tcp"),
			},
		},
		Statics: []appconfig.Static{{
			GuestPath: "/app/assets",
			UrlPrefix: "/statics",
		}},
		Services: []appconfig.Service{{
			Protocol:     "tcp",
			InternalPort: 8080,
		}},
	})
	require.NoError(t, err)

	md.volumes = map[string][]api.Volume{
		"data": {{ID: "vol_12345"}},
	}

	// New app machine
	li, err := md.launchInputForLaunch("", nil)
	require.NoError(t, err)
	assert.Equal(t, &api.LaunchMachineInput{
		OrgSlug: "my-dangling-org",
		Config: &api.MachineConfig{
			Env: map[string]string{
				"PRIMARY_REGION":    "scl",
				"OTHER":             "value",
				"FLY_PROCESS_GROUP": "app",
			},
			Image: "super/balloon",
			Metadata: map[string]string{
				"fly_platform_version": "v2",
				"fly_process_group":    "app",
				"fly_release_id":       "",
				"fly_release_version":  "0",
			},
			Metrics: &api.MachineMetrics{
				Port: 9000,
				Path: "/prometheus",
			},
			Mounts: []api.MachineMount{{
				Name:   "data",
				Volume: "vol_12345",
				Path:   "/data",
			}},
			Statics: []*api.Static{{
				GuestPath: "/app/assets",
				UrlPrefix: "/statics",
			}},
			Services: []api.MachineService{{
				Protocol:     "tcp",
				InternalPort: 8080,
			}},
			Checks: map[string]api.MachineCheck{
				"alive": {
					Port: api.Pointer(8080),
					Type: api.Pointer("tcp"),
				},
			},
		},
	}, li)

	// New release command machine
	assert.Equal(t, &api.LaunchMachineInput{
		OrgSlug: "my-dangling-org",
		Config: &api.MachineConfig{
			Init: api.MachineInit{
				Cmd: []string{"touch", "sky"},
			},
			Env: map[string]string{
				"PRIMARY_REGION":    "scl",
				"OTHER":             "value",
				"RELEASE_COMMAND":   "1",
				"FLY_PROCESS_GROUP": "fly_app_release_command",
			},
			Image: "super/balloon",
			Metadata: map[string]string{
				"fly_platform_version": "v2",
				"fly_process_group":    "fly_app_release_command",
				"fly_release_id":       "",
				"fly_release_version":  "0",
			},
			Restart: api.MachineRestart{
				Policy: api.MachineRestartPolicyNo,
			},
			AutoDestroy: true,
			DNS: &api.DNSConfig{
				SkipRegistration: true,
			},
			Guest: api.MachinePresets["shared-cpu-2x"],
		},
	}, md.launchInputForReleaseCommand(nil))

	// Update existing release command machine
	origMachine := &api.Machine{
		Config: &api.MachineConfig{
			Env: map[string]string{
				"PRIMARY_REGION": "different-region",
			},
			AutoDestroy: false,
			Restart: api.MachineRestart{
				Policy: api.MachineRestartPolicyOnFailure,
			},
			Init: api.MachineInit{
				Cmd: []string{"touch", "ground"},
			},
		},
	}
	assert.Equal(t, &api.LaunchMachineInput{
		OrgSlug: "my-dangling-org",
		Config: &api.MachineConfig{
			Env: map[string]string{
				"PRIMARY_REGION":    "scl",
				"OTHER":             "value",
				"RELEASE_COMMAND":   "1",
				"FLY_PROCESS_GROUP": "fly_app_release_command",
			},
			Image: "super/balloon",
			Metadata: map[string]string{
				"fly_platform_version": "v2",
				"fly_process_group":    "fly_app_release_command",
				"fly_release_id":       "",
				"fly_release_version":  "0",
			},
			Init: api.MachineInit{
				Cmd: []string{"touch", "sky"},
			},
			Restart: api.MachineRestart{
				Policy: api.MachineRestartPolicyNo,
			},
			AutoDestroy: true,
			DNS: &api.DNSConfig{
				SkipRegistration: true,
			},
			Guest: api.MachinePresets["shared-cpu-2x"],
		},
	}, md.launchInputForReleaseCommand(origMachine))
}

// Test Mounts
func Test_resolveUpdatedMachineConfig_Mounts(t *testing.T) {
	md, err := stabMachineDeployment(&appconfig.Config{
		Mounts: []appconfig.Mount{{
			Source:      "data",
			Destination: "/data",
		}},
	})
	require.NoError(t, err)
	md.volumes = map[string][]api.Volume{
		"data": {{ID: "vol_12345"}},
	}

	// New app machine
	li, err := md.launchInputForLaunch("", nil)
	require.NoError(t, err)
	assert.Equal(t, &api.LaunchMachineInput{
		OrgSlug: "my-dangling-org",
		Config: &api.MachineConfig{
			Image: "super/balloon",
			Metadata: map[string]string{
				"fly_platform_version": "v2",
				"fly_process_group":    "app",
				"fly_release_id":       "",
				"fly_release_version":  "0",
			},
			Env: map[string]string{
				"FLY_PROCESS_GROUP": "app",
			},
			Mounts: []api.MachineMount{{
				Volume: "vol_12345",
				Path:   "/data",
				Name:   "data",
			}},
		},
	}, li)

	origMachine := &api.Machine{
		Config: &api.MachineConfig{
			Mounts: []api.MachineMount{{
				Volume: "vol_alreadyattached",
				Path:   "/data",
			}},
		},
	}

	// Reuse app machine
	li, err = md.launchInputForUpdate(origMachine)
	require.NoError(t, err)
	assert.Equal(t, &api.LaunchMachineInput{
		OrgSlug: "my-dangling-org",
		Config: &api.MachineConfig{
			Image: "super/balloon",
			Metadata: map[string]string{
				"fly_platform_version": "v2",
				"fly_process_group":    "app",
				"fly_release_id":       "",
				"fly_release_version":  "0",
			},
			Env: map[string]string{
				"FLY_PROCESS_GROUP": "app",
			},
			Mounts: []api.MachineMount{{
				Volume: "vol_alreadyattached",
				Path:   "/data",
			}},
		},
	}, li)
}

// Test machineDeployment.restartOnly
func Test_resolveUpdatedMachineConfig_restartOnly(t *testing.T) {
	md, err := stabMachineDeployment(&appconfig.Config{
		Env: map[string]string{
			"Ignore": "me",
		},
		Mounts: []appconfig.Mount{{
			Source:      "data",
			Destination: "/data",
		}},
	})
	assert.NoError(t, err)
	md.img = "SHOULD-NOT-USE-THIS-TAG"

	origMachine := &api.Machine{
		ID: "OrigID",
		Config: &api.MachineConfig{
			Image: "instead-use/the-redmoon",
		},
	}

	assert.Equal(t, &api.LaunchMachineInput{
		ID:      "OrigID",
		OrgSlug: "my-dangling-org",
		Config: &api.MachineConfig{
			Image: "instead-use/the-redmoon",
			Metadata: map[string]string{
				"fly_platform_version": "v2",
				"fly_process_group":    "app",
				"fly_release_id":       "",
				"fly_release_version":  "0",
			},
		},
	}, md.launchInputForRestart(origMachine))
}

// Test machineDeployment.restartOnlyProcessGroup
func Test_resolveUpdatedMachineConfig_restartOnlyProcessGroup(t *testing.T) {
	md, err := stabMachineDeployment(&appconfig.Config{
		Env: map[string]string{
			"Ignore": "me",
		},
		Mounts: []appconfig.Mount{{
			Source:      "data",
			Destination: "/data",
		}},
	})
	md.releaseVersion = 2
	assert.NoError(t, err)
	md.img = "SHOULD-NOT-USE-THIS-TAG"

	origMachine := &api.Machine{
		ID: "OrigID",
		Config: &api.MachineConfig{
			Image: "instead-use/the-redmoon",
			Metadata: map[string]string{
				"fly_process_group":   "awesome-group",
				"fly_release_version": "1",
				// The app isn't managed postgres, so this
				// should end up stripped out.
				"fly-managed-postgres": "true",
			},
		},
	}

	assert.Equal(t, &api.LaunchMachineInput{
		ID:      "OrigID",
		OrgSlug: "my-dangling-org",
		Config: &api.MachineConfig{
			Image: "instead-use/the-redmoon",
			Metadata: map[string]string{
				"fly_platform_version": "v2",
				"fly_process_group":    "awesome-group",
				"fly_release_id":       "",
				"fly_release_version":  "2",
			},
		},
	}, md.launchInputForRestart(origMachine))
}
