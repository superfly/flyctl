package deploy

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/internal/appv2"
	"github.com/superfly/flyctl/internal/build/imgsrc"
)

func Test_resultUpdateMachineConfig_Basic(t *testing.T) {
	md := &machineDeployment{
		app: &api.AppCompact{
			ID: "my-cool-app",
			Organization: &api.OrganizationBasic{
				ID: "my-dangling-org",
			},
		},
		img: &imgsrc.DeploymentImage{
			Tag: "super/ballon",
		},
		appConfig: &appv2.Config{
			AppName: "my-cool-app",
			Env: map[string]string{
				"PRIMARY_REGION": "scl",
				"OTHER":          "value",
			},
		},
	}
	var err error
	md.processConfigs, err = md.appConfig.GetProcessConfigs()
	assert.NoError(t, err)

	launchInput := md.resolveUpdatedMachineConfig(nil, false)

	assert.Equal(t, &api.LaunchMachineInput{
		OrgSlug: "my-dangling-org",
		Config: &api.MachineConfig{
			Env: map[string]string{
				"PRIMARY_REGION": "scl",
				"OTHER":          "value",
			},
			Image: "super/ballon",
			Metadata: map[string]string{
				"fly_platform_version": "v2",
				"fly_process_group":    "app",
				"fly_release_id":       "",
				"fly_release_version":  "0",
			},
			Services: []api.MachineService{},
			Checks:   map[string]api.MachineCheck{},
		},
	}, launchInput)
}

func Test_resultUpdateMachineConfig_RelaseCommand(t *testing.T) {
	md := &machineDeployment{
		app: &api.AppCompact{
			ID: "my-cool-app",
			Organization: &api.OrganizationBasic{
				ID: "my-dangling-org",
			},
		},
		img: &imgsrc.DeploymentImage{
			Tag: "super/ballon",
		},
		volumes: []api.Volume{
			{ID: "vol_12345"},
		},
		volumeDestination: "/data",
		appConfig: &appv2.Config{
			AppName: "my-cool-app",
			Env: map[string]string{
				"PRIMARY_REGION": "scl",
				"OTHER":          "value",
			},
			Metrics: &api.MachineMetrics{
				Port: 9000,
				Path: "/prometheus",
			},
			Deploy: &appv2.Deploy{
				ReleaseCommand: "echo foo",
			},
			Mounts: &appv2.Volume{
				Source:      "data",
				Destination: "/data",
			},
			Checks: map[string]*appv2.ToplevelCheck{
				"alive": {
					Port: api.Pointer(8080),
					Type: api.Pointer("tcp"),
				},
			},
			Statics: []appv2.Static{{
				GuestPath: "/app/assets",
				UrlPrefix: "/statics",
			}},
			Services: []appv2.Service{{
				Protocol:     "tcp",
				InternalPort: 8080,
			}},
		},
	}
	var err error
	md.processConfigs, err = md.appConfig.GetProcessConfigs()
	assert.NoError(t, err)

	// New app machine
	assert.Equal(t, &api.LaunchMachineInput{
		OrgSlug: "my-dangling-org",
		Config: &api.MachineConfig{
			Env: map[string]string{
				"PRIMARY_REGION": "scl",
				"OTHER":          "value",
			},
			Image: "super/ballon",
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
	},
		md.resolveUpdatedMachineConfig(nil, false),
	)

	// New release command machine
	assert.Equal(t, &api.LaunchMachineInput{
		OrgSlug: "my-dangling-org",
		Config: &api.MachineConfig{
			Env: map[string]string{
				"PRIMARY_REGION": "scl",
				"OTHER":          "value",
			},
			Image: "super/ballon",
			Metadata: map[string]string{
				"fly_platform_version": "v2",
				"fly_process_group":    "app",
				"fly_release_id":       "",
				"fly_release_version":  "0",
			},
		},
	},
		md.resolveUpdatedMachineConfig(nil, true),
	)
}
