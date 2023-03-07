package deploy

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/build/imgsrc"
)

func stabMachineDeployment(appConfig *appconfig.Config) (*machineDeployment, error) {
	md := &machineDeployment{
		app: &api.AppCompact{
			ID: "my-cool-app",
			Organization: &api.OrganizationBasic{
				ID: "my-dangling-org",
			},
		},
		img: &imgsrc.DeploymentImage{
			Tag: "super/balloon",
		},
		appConfig: appConfig,
	}
	var err error
	md.processConfigs, err = md.appConfig.GetProcessConfigs()
	return md, err
}

func Test_resultUpdateMachineConfig_Basic(t *testing.T) {
	md, err := stabMachineDeployment(&appconfig.Config{
		AppName: "my-cool-app",
		Env: map[string]string{
			"PRIMARY_REGION": "scl",
			"OTHER":          "value",
		},
	})
	assert.NoError(t, err)
	assert.Equal(t, &api.LaunchMachineInput{
		OrgSlug: "my-dangling-org",
		Config: &api.MachineConfig{
			Env: map[string]string{
				"PRIMARY_REGION": "scl",
				"OTHER":          "value",
			},
			Image: "super/balloon",
			Metadata: map[string]string{
				"fly_platform_version": "v2",
				"fly_process_group":    "app",
				"fly_release_id":       "",
				"fly_release_version":  "0",
			},
			Services: []api.MachineService{},
			Checks:   map[string]api.MachineCheck{},
		},
	}, md.resolveUpdatedMachineConfig(nil, false))
}

// Test any LaunchMachineInput field that must not be set on a machine
// used to run release command.
func Test_resultUpdateMachineConfig_ReleaseCommand(t *testing.T) {
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
			ReleaseCommand: "echo foo",
		},
		Mounts: &appconfig.Volume{
			Source:      "data",
			Destination: "/data",
		},
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
	assert.NoError(t, err)
	md.volumes = []api.Volume{{ID: "vol_12345"}}
	md.volumeDestination = "/data"
	md.releaseCommand = []string{"touch", "sky"}

	// New app machine
	assert.Equal(t, &api.LaunchMachineInput{
		OrgSlug: "my-dangling-org",
		Config: &api.MachineConfig{
			Env: map[string]string{
				"PRIMARY_REGION": "scl",
				"OTHER":          "value",
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
				Checks:       []api.MachineCheck{},
			}},
			Checks: map[string]api.MachineCheck{
				"alive": {
					Port:        api.Pointer(8080),
					Type:        api.Pointer("tcp"),
					HTTPHeaders: []api.MachineHTTPHeader{},
				},
			},
		},
	}, md.resolveUpdatedMachineConfig(nil, false))

	// New release command machine
	assert.Equal(t, &api.LaunchMachineInput{
		OrgSlug: "my-dangling-org",
		Config: &api.MachineConfig{
			Init: api.MachineInit{
				Cmd: []string{"touch", "sky"},
			},
			Env: map[string]string{
				"PRIMARY_REGION":  "scl",
				"OTHER":           "value",
				"RELEASE_COMMAND": "1",
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
		},
	}, md.resolveUpdatedMachineConfig(nil, true))

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
				"PRIMARY_REGION":  "scl",
				"OTHER":           "value",
				"RELEASE_COMMAND": "1",
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
		},
	}, md.resolveUpdatedMachineConfig(origMachine, true))
}

// Test Mounts
func Test_resultUpdateMachineConfig_Mounts(t *testing.T) {
	md, err := stabMachineDeployment(&appconfig.Config{
		Mounts: &appconfig.Volume{
			Source:      "data",
			Destination: "/data",
		},
	})
	assert.NoError(t, err)
	md.volumeDestination = "/data"
	md.volumes = []api.Volume{{ID: "vol_12345"}}

	// New app machine
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
			Env:      map[string]string{},
			Services: []api.MachineService{},
			Checks:   map[string]api.MachineCheck{},
			Mounts: []api.MachineMount{{
				Volume: "vol_12345",
				Path:   "/data",
			}},
		},
	},
		md.resolveUpdatedMachineConfig(nil, false),
	)

	origMachine := &api.Machine{
		Config: &api.MachineConfig{
			Mounts: []api.MachineMount{{
				Volume: "vol_alreadyattached",
				Path:   "/data",
			}},
		},
	}

	// Reuse app machine
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
			Env:      map[string]string{},
			Services: []api.MachineService{},
			Checks:   map[string]api.MachineCheck{},
			Mounts: []api.MachineMount{{
				Volume: "vol_alreadyattached",
				Path:   "/data",
			}},
		},
	}, md.resolveUpdatedMachineConfig(origMachine, false))
}

// Test machineDeployment.restartOnly
func Test_resultUpdateMachineConfig_restartOnly(t *testing.T) {
	md, err := stabMachineDeployment(&appconfig.Config{
		Env: map[string]string{
			"Ignore": "me",
		},
		Mounts: &appconfig.Volume{
			Source:      "data",
			Destination: "/data",
		},
	})
	assert.NoError(t, err)
	md.restartOnly = true
	md.img.Tag = "SHOULD-NOT-USE-THIS-TAG"

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
	}, md.resolveUpdatedMachineConfig(origMachine, false))
}
