package deploy

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/buildinfo"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/machine"
	"github.com/superfly/flyctl/internal/mock"
	"github.com/superfly/flyctl/iostreams"
)

func stabMachineDeployment(appConfig *appconfig.Config) (*machineDeployment, error) {
	md := &machineDeployment{
		app: &flaps.App{
			Name: "my-cool-app",
			Organization: flaps.AppOrganizationInfo{
				Slug: "my-org",
			},
		},
		img:        "super/balloon",
		appConfig:  appConfig,
		machineSet: machine.NewMachineSet(nil, nil, "", nil, true),
	}

	return md, nil
}

type volumeListFlapsClient struct {
	flapsutil.FlapsClient
	volumes        []fly.Volume
	createRequests []fly.CreateVolumeRequest
}

func (c *volumeListFlapsClient) GetVolumes(context.Context, string) ([]fly.Volume, error) {
	return c.volumes, nil
}

func (c *volumeListFlapsClient) CreateVolume(_ context.Context, _ string, request fly.CreateVolumeRequest) (*fly.Volume, error) {
	c.createRequests = append(c.createRequests, request)

	return &fly.Volume{Name: request.Name, Region: request.Region, State: "created", HostStatus: "ok"}, nil
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
		MinSecretsVersion: nil,
	}, li)
}

func TestSetMachinesForDeploymentRejectsDetachedProcessGroupMachinesForBluegreen(t *testing.T) {
	detachedMachine := testProcessGroupMachine("detached-app", "", map[string]string{
		fly.MachineConfigMetadataKeyFlyProcessGroup: fly.MachineProcessGroupApp,
	})
	md := testMachineDeploymentForSetMachines(t, "bluegreen", nil, []*fly.Machine{detachedMachine})

	err := md.setMachinesForDeployment(context.Background())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "detached-app")
	assert.Contains(t, err.Error(), fly.MachineConfigMetadataKeyFlyPlatformVersion)
}

func TestSetMachinesForDeploymentRejectsDetachedDefaultProcessGroupMachinesForBluegreen(t *testing.T) {
	detachedMachine := testProcessGroupMachine("detached-app", "", nil)
	md := testMachineDeploymentForSetMachines(t, "bluegreen", nil, []*fly.Machine{detachedMachine})

	err := md.setMachinesForDeployment(context.Background())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "detached-app")
	assert.Contains(t, err.Error(), fly.MachineConfigMetadataKeyFlyProcessGroup)
}

func TestSetMachinesForDeploymentAllowsDetachedProcessGroupMachinesForBluegreenWithManagedMachines(t *testing.T) {
	managedMachine := testFlyLaunchMachine("managed-app")
	detachedMachine := testProcessGroupMachine("detached-app", "", map[string]string{
		fly.MachineConfigMetadataKeyFlyProcessGroup: fly.MachineProcessGroupApp,
	})
	md := testMachineDeploymentForSetMachines(t, "bluegreen", []*fly.Machine{managedMachine}, []*fly.Machine{managedMachine, detachedMachine})

	err := md.setMachinesForDeployment(context.Background())

	require.NoError(t, err)
	machines := md.machineSet.GetMachines()
	require.Len(t, machines, 1)
	assert.Equal(t, "managed-app", machines[0].Machine().ID)
}

func TestSetMachinesForDeploymentAllowsDetachedProcessGroupMachinesForRolling(t *testing.T) {
	detachedMachine := testProcessGroupMachine("detached-app", "", map[string]string{
		fly.MachineConfigMetadataKeyFlyProcessGroup: fly.MachineProcessGroupApp,
	})
	md := testMachineDeploymentForSetMachines(t, "rolling", nil, []*fly.Machine{detachedMachine})

	err := md.setMachinesForDeployment(context.Background())

	require.NoError(t, err)
	assert.True(t, md.isFirstDeploy)
}

func TestSetMachinesForDeploymentDoesNotReportFilteredFlyLaunchMachinesAsDetached(t *testing.T) {
	managedMachine := testFlyLaunchMachine("managed-app")
	managedMachine.Region = "ord"
	detachedMachine := testProcessGroupMachine("detached-app", "iad", map[string]string{
		fly.MachineConfigMetadataKeyFlyProcessGroup: fly.MachineProcessGroupApp,
	})
	md := testMachineDeploymentForSetMachines(t, "bluegreen", []*fly.Machine{managedMachine}, []*fly.Machine{managedMachine, detachedMachine})
	md.onlyRegions = map[string]bool{"iad": true}

	err := md.setMachinesForDeployment(context.Background())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "detached-app")
	assert.NotContains(t, err.Error(), "managed-app")
}

func TestSetMachinesForDeploymentIgnoresDetachedProcessGroupMachinesOutsideBluegreenFilters(t *testing.T) {
	detachedMachine := testProcessGroupMachine("detached-app", "lax", map[string]string{
		fly.MachineConfigMetadataKeyFlyProcessGroup: fly.MachineProcessGroupApp,
	})
	md := testMachineDeploymentForSetMachines(t, "bluegreen", nil, []*fly.Machine{detachedMachine})
	md.onlyRegions = map[string]bool{"iad": true}

	err := md.setMachinesForDeployment(context.Background())

	require.NoError(t, err)
	assert.True(t, md.isFirstDeploy)
}

func testFlyLaunchMachine(id string) *fly.Machine {
	return testProcessGroupMachine(id, "", map[string]string{
		fly.MachineConfigMetadataKeyFlyPlatformVersion: fly.MachineFlyPlatformVersion2,
		fly.MachineConfigMetadataKeyFlyProcessGroup:    fly.MachineProcessGroupApp,
	})
}

func testProcessGroupMachine(id, region string, metadata map[string]string) *fly.Machine {
	return &fly.Machine{
		ID:     id,
		Region: region,
		State:  fly.MachineStateStarted,
		Config: &fly.MachineConfig{
			Metadata: metadata,
		},
	}
}

func testMachineDeploymentForSetMachines(t *testing.T, strategy string, flyLaunchMachines, activeMachines []*fly.Machine) *machineDeployment {
	t.Helper()

	ios, _, _, _ := iostreams.Test()
	client := &mock.FlapsClient{
		ListFlyAppsMachinesFunc: func(ctx context.Context, appName string) ([]*fly.Machine, *fly.Machine, error) {
			return flyLaunchMachines, nil, nil
		},
		ListActiveFunc: func(ctx context.Context, appName string) ([]*fly.Machine, error) {
			return activeMachines, nil
		},
	}

	return &machineDeployment{
		app:         &flaps.App{Name: "my-cool-app"},
		io:          ios,
		colorize:    ios.ColorScheme(),
		flapsClient: client,
		strategy:    strategy,
		appConfig: &appconfig.Config{
			HTTPService: &appconfig.HTTPService{
				Processes: []string{fly.MachineProcessGroupApp},
			},
		},
	}
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
				Port: new(8080),
				Type: new("tcp"),
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
				Volume: "data",
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
					Port: new(8080),
					Type: new("tcp"),
				},
			},
		},
		MinSecretsVersion: nil,
	}, li)

	got, err := md.launchInputForReleaseCommand(nil)
	assert.NoError(t, err)

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
		SkipLaunch:        true,
		MinSecretsVersion: nil,
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

	got, err = md.launchInputForReleaseCommand(origMachine)
	assert.NoError(t, err)

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
		SkipLaunch:        true,
		MinSecretsVersion: nil,
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
				Volume: "data",
				Path:   "/data",
				Name:   "data",
			}},
		},
		MinSecretsVersion: nil,
	}, li)

	origMachine := &fly.Machine{
		State:      fly.MachineStateStarted,
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
		MinSecretsVersion: nil,
	}, li)
}

func TestValidateVolumeConfigNewGroupRequiresVolumeInPrimaryRegion(t *testing.T) {
	md, err := stabMachineDeployment(&appconfig.Config{
		PrimaryRegion: "qmx",
		Processes:     map[string]string{"worker": "/bin/worker"},
		Mounts: []appconfig.Mount{{
			Source:      "data",
			Destination: "/data",
			Processes:   []string{"worker"},
		}},
	})
	require.NoError(t, err)
	md.availableVolumeCounts = map[string]map[string]int{"data": {"syd": 1}}

	err = md.validateVolumeConfig(context.Background())
	require.ErrorContains(t, err, "requires an unattached 'data' volume in region 'qmx'")
	require.ErrorContains(t, err, "fly volume create data --region qmx")

	md.availableVolumeCounts["data"]["qmx"] = 1
	require.NoError(t, md.validateVolumeConfig(context.Background()))
}

func TestValidateVolumeConfigReservesSharedVolumesAcrossNewGroups(t *testing.T) {
	md, err := stabMachineDeployment(&appconfig.Config{
		PrimaryRegion: "qmx",
		Processes: map[string]string{
			"alpha": "/bin/alpha",
			"beta":  "/bin/beta",
		},
		Mounts: []appconfig.Mount{{
			Source:      "shared_data",
			Destination: "/data",
			Processes:   []string{"alpha", "beta"},
		}},
	})
	require.NoError(t, err)
	md.availableVolumeCounts = map[string]map[string]int{"shared_data": {"qmx": 1}}

	err = md.validateVolumeConfig(context.Background())
	require.ErrorContains(t, err, "group 'beta' requires an unattached 'shared_data' volume")

	md.availableVolumeCounts["shared_data"]["qmx"] = 2
	require.NoError(t, md.validateVolumeConfig(context.Background()))
}

func TestValidateVolumeConfigReservesSharedVolumesAcrossExistingAndNewGroups(t *testing.T) {
	md, err := stabMachineDeployment(&appconfig.Config{
		PrimaryRegion: "qmx",
		Processes: map[string]string{
			"alpha": "/bin/alpha",
			"beta":  "/bin/beta",
		},
		Mounts: []appconfig.Mount{{
			Source:      "shared_data",
			Destination: "/data",
			Processes:   []string{"alpha", "beta"},
		}},
	})
	require.NoError(t, err)
	ios, _, _, _ := iostreams.Test()
	md.io = ios
	md.colorize = ios.ColorScheme()
	md.machineSet = machine.NewMachineSet(nil, ios, "", []*fly.Machine{{
		ID:     "machine-alpha",
		Region: "qmx",
		Config: &fly.MachineConfig{
			Metadata: map[string]string{fly.MachineConfigMetadataKeyFlyProcessGroup: "alpha"},
		},
	}}, true)
	md.availableVolumeCounts = map[string]map[string]int{"shared_data": {"qmx": 1}}

	err = md.validateVolumeConfig(context.Background())
	require.ErrorContains(t, err, "group 'beta' requires an unattached 'shared_data' volume")

	md.availableVolumeCounts["shared_data"]["qmx"] = 2
	require.NoError(t, md.validateVolumeConfig(context.Background()))
}

func TestSetVolumesOnlyCountsResolvableVolumes(t *testing.T) {
	attachedMachine := "machine-id"
	attachedAllocation := "allocation-id"
	client := &volumeListFlapsClient{volumes: []fly.Volume{
		{Name: "data", Region: "qmx", State: "created", HostStatus: "ok"},
		{Name: "data", Region: "syd", State: "created", HostStatus: "ok"},
		{Name: "data", Region: "qmx", State: "pending_destroy", HostStatus: "ok"},
		{Name: "data", Region: "qmx", State: "created", HostStatus: "unreachable"},
		{Name: "data", Region: "qmx", State: "created", HostStatus: "ok", AttachedMachine: &attachedMachine},
		{Name: "data", Region: "qmx", State: "created", HostStatus: "ok", AttachedAllocation: &attachedAllocation},
	}}
	md, err := stabMachineDeployment(&appconfig.Config{Mounts: []appconfig.Mount{{Source: "data", Destination: "/data"}}})
	require.NoError(t, err)
	md.flapsClient = client

	require.NoError(t, md.setVolumes(context.Background()))
	require.Equal(t, map[string]map[string]int{"data": {"qmx": 1, "syd": 1}}, md.availableVolumeCounts)
}

func TestProvisionVolumesOnFirstDeployReservesByNameAndRegion(t *testing.T) {
	client := &volumeListFlapsClient{}
	md, err := stabMachineDeployment(&appconfig.Config{
		PrimaryRegion: "qmx",
		Processes: map[string]string{
			"alpha": "/bin/alpha",
			"beta":  "/bin/beta",
		},
		Mounts: []appconfig.Mount{{
			Source:      "shared_data",
			Destination: "/data",
			Processes:   []string{"alpha", "beta"},
		}},
	})
	require.NoError(t, err)
	ios, _, _, _ := iostreams.Test()
	md.io = ios
	md.colorize = ios.ColorScheme()
	md.flapsClient = client
	md.isFirstDeploy = true
	md.availableVolumeCounts = map[string]map[string]int{"shared_data": {"qmx": 1, "syd": 1}}

	require.NoError(t, md.provisionVolumesOnFirstDeploy(context.Background()))
	require.Len(t, client.createRequests, 1)
	require.Equal(t, "shared_data", client.createRequests[0].Name)
	require.Equal(t, "qmx", client.createRequests[0].Region)
	require.Equal(t, 2, md.availableVolumeCounts["shared_data"]["qmx"])
}

func TestValidateVolumeConfigReplacementChecksCountByRegion(t *testing.T) {
	md, err := stabMachineDeployment(&appconfig.Config{
		PrimaryRegion: "qmx",
		Processes:     map[string]string{"worker": "/bin/worker"},
		Mounts: []appconfig.Mount{{
			Source:      "data",
			Destination: "/data",
			Processes:   []string{"worker"},
		}},
	})
	require.NoError(t, err)
	machines := []*fly.Machine{
		{ID: "machine-1", Region: "qmx", Config: &fly.MachineConfig{Metadata: map[string]string{fly.MachineConfigMetadataKeyFlyProcessGroup: "worker"}}},
		{ID: "machine-2", Region: "qmx", Config: &fly.MachineConfig{Metadata: map[string]string{fly.MachineConfigMetadataKeyFlyProcessGroup: "worker"}}},
	}
	ios, _, _, _ := iostreams.Test()
	md.io = ios
	md.colorize = ios.ColorScheme()
	md.machineSet = machine.NewMachineSet(nil, ios, "", machines, true)
	md.availableVolumeCounts = map[string]map[string]int{"data": {"qmx": 1, "syd": 2}}

	err = md.validateVolumeConfig(context.Background())
	require.ErrorContains(t, err, "Process group 'worker' needs volumes with name 'data'")
	require.ErrorContains(t, err, "qmx=1")

	md.availableVolumeCounts["data"]["qmx"] = 2
	require.NoError(t, md.validateVolumeConfig(context.Background()))
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
		State:      fly.MachineStateStarted,
		HostStatus: fly.HostStatusOk,
		ID:         "OrigID",
		Config: &fly.MachineConfig{
			Image: "instead-use/the-redmoon",
		},
	}

	got, err := md.launchInputForRestart(origMachine)
	assert.NoError(t, err)

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
		MinSecretsVersion: nil,
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
		State:      fly.MachineStateStarted,
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

	got, err := md.launchInputForRestart(origMachine)
	assert.NoError(t, err)

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
		MinSecretsVersion: nil,
	}, got)
}
