package deploy

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/buildinfo"
	"github.com/superfly/flyctl/internal/machine"
)

func stabMachineDeployment(appConfig *appconfig.Config) (*machineDeployment, error) {
	md := &machineDeployment{
		app: &fly.AppCompact{
			ID: "my-cool-app",
			Organization: &fly.OrganizationBasic{
				ID: "my-dangling-org",
			},
		},
		img:        "super/balloon",
		appConfig:  appConfig,
		machineSet: machine.NewMachineSet(nil, nil, nil, true),
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
	li, err := md.launchInputForLaunch("", nil, nil)
	require.NoError(t, err)

	assert.Equal(t, &fly.LaunchMachineInput{
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
				"fly_release_id":       "",
				"fly_release_version":  "0",
				"fly_flyctl_version":   buildinfo.Version().String(),
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
		Metrics: []*appconfig.Metrics{
			{
				MachineMetrics: &fly.MachineMetrics{
					Port: 9000,
					Path: "/prometheus",
				},
			},
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
				Port: fly.Pointer(8080),
				Type: fly.Pointer("tcp"),
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

	md.volumes = map[string][]fly.Volume{
		"data": {{ID: "vol_12345"}},
	}

	// New app machine
	li, err := md.launchInputForLaunch("", nil, nil)
	require.NoError(t, err)

	assert.Equal(t, &fly.LaunchMachineInput{
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
				"fly_release_id":       "",
				"fly_release_version":  "0",
				"fly_flyctl_version":   buildinfo.Version().String(),
			},
			Metrics: &fly.MachineMetrics{
				Port: 9000,
				Path: "/prometheus",
			},
			Mounts: []fly.MachineMount{{
				Name:   "data",
				Volume: "vol_12345",
				Path:   "/data",
			}},
			Statics: []*fly.Static{{
				GuestPath: "/app/assets",
				UrlPrefix: "/statics",
			}},
			Services: []fly.MachineService{{
				Protocol:     "tcp",
				InternalPort: 8080,
			}},
			Checks: map[string]fly.MachineCheck{
				"alive": {
					Port: fly.Pointer(8080),
					Type: fly.Pointer("tcp"),
				},
			},
		},
	}, li)

	got := md.launchInputForReleaseCommand(nil)

	// New release command machine
	assert.Equal(t, &fly.LaunchMachineInput{
		Config: &fly.MachineConfig{
			Init: fly.MachineInit{
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
				"fly_flyctl_version":   buildinfo.Version().String(),
			},
			Restart: &fly.MachineRestart{
				Policy: fly.MachineRestartPolicyNo,
			},
			AutoDestroy: true,
			DNS: &fly.DNSConfig{
				SkipRegistration: true,
			},
			Guest: fly.MachinePresets["shared-cpu-2x"],
		},
		SkipLaunch: true,
	}, got)

	// Update existing release command machine
	origMachine := &fly.Machine{
		HostStatus: fly.HostStatusOk,
		Config: &fly.MachineConfig{
			Env: map[string]string{
				"PRIMARY_REGION": "different-region",
			},
			AutoDestroy: false,
			Restart: &fly.MachineRestart{
				Policy: fly.MachineRestartPolicyOnFailure,
			},
			Init: fly.MachineInit{
				Cmd: []string{"touch", "ground"},
			},
		},
	}

	got = md.launchInputForReleaseCommand(origMachine)

	assert.Equal(t, &fly.LaunchMachineInput{
		Config: &fly.MachineConfig{
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
				"fly_flyctl_version":   buildinfo.Version().String(),
			},
			Init: fly.MachineInit{
				Cmd: []string{"touch", "sky"},
			},
			Restart: &fly.MachineRestart{
				Policy: fly.MachineRestartPolicyNo,
			},
			AutoDestroy: true,
			DNS: &fly.DNSConfig{
				SkipRegistration: true,
			},
			Guest: fly.MachinePresets["shared-cpu-2x"],
		},
		SkipLaunch: true,
	}, got)
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
	md.volumes = map[string][]fly.Volume{
		"data": {{ID: "vol_12345"}},
	}

	// New app machine
	li, err := md.launchInputForLaunch("", nil, nil)
	require.NoError(t, err)

	assert.Equal(t, &fly.LaunchMachineInput{
		Config: &fly.MachineConfig{
			Image: "super/balloon",
			Metadata: map[string]string{
				"fly_platform_version": "v2",
				"fly_process_group":    "app",
				"fly_release_id":       "",
				"fly_release_version":  "0",
				"fly_flyctl_version":   buildinfo.Version().String(),
			},
			Env: map[string]string{
				"FLY_PROCESS_GROUP": "app",
			},
			Mounts: []fly.MachineMount{{
				Volume: "vol_12345",
				Path:   "/data",
				Name:   "data",
			}},
		},
	}, li)

	origMachine := &fly.Machine{
		HostStatus: fly.HostStatusOk,
		Config: &fly.MachineConfig{
			Mounts: []fly.MachineMount{{
				Volume: "vol_alreadyattached",
				Path:   "/data",
			}},
		},
	}

	// Reuse app machine
	li, err = md.launchInputForUpdate(origMachine)
	require.NoError(t, err)

	assert.Equal(t, &fly.LaunchMachineInput{
		Config: &fly.MachineConfig{
			Image: "super/balloon",
			Metadata: map[string]string{
				"fly_platform_version": "v2",
				"fly_process_group":    "app",
				"fly_release_id":       "",
				"fly_release_version":  "0",
				"fly_flyctl_version":   buildinfo.Version().String(),
			},
			Env: map[string]string{
				"FLY_PROCESS_GROUP": "app",
			},
			Mounts: []fly.MachineMount{{
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

	origMachine := &fly.Machine{
		HostStatus: fly.HostStatusOk,
		ID:         "OrigID",
		Config: &fly.MachineConfig{
			Image: "instead-use/the-redmoon",
		},
	}

	got := md.launchInputForRestart(origMachine)

	assert.Equal(t, &fly.LaunchMachineInput{
		ID: "OrigID",
		Config: &fly.MachineConfig{
			Image: "instead-use/the-redmoon",
			Metadata: map[string]string{
				"fly_platform_version": "v2",
				"fly_process_group":    "app",
				"fly_release_id":       "",
				"fly_release_version":  "0",
				"fly_flyctl_version":   buildinfo.Version().String(),
			},
		},
	}, got)
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

	origMachine := &fly.Machine{
		HostStatus: fly.HostStatusOk,
		ID:         "OrigID",
		Config: &fly.MachineConfig{
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

	got := md.launchInputForRestart(origMachine)
	assert.Equal(t, &fly.LaunchMachineInput{
		ID: "OrigID",
		Config: &fly.MachineConfig{
			Image: "instead-use/the-redmoon",
			Metadata: map[string]string{
				"fly_platform_version": "v2",
				"fly_process_group":    "awesome-group",
				"fly_release_id":       "",
				"fly_release_version":  "2",
				"fly_flyctl_version":   buildinfo.Version().String(),
			},
		},
	}, got)
}
