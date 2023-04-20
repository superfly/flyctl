//go:build integration
// +build integration

package preflight

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/test/preflight/testlib"
	"golang.org/x/exp/slices"
)

// test --port and --autostart --autostop flags
func TestFlyMachineRun_case01(t *testing.T) {
	f := testlib.NewTestEnvFromEnv(t)
	appName := f.CreateRandomAppMachines()

	f.Fly("machine run -a %s nginx --port 80:81 --autostop", appName)
	ml := f.MachinesList(appName)
	require.Equal(f, 1, len(ml))

	m := ml[0]
	want := []api.MachineService{{
		Protocol:     "tcp",
		InternalPort: 81,
		Autostop:     api.Pointer(true),
		Ports: []api.MachinePort{{
			Port:       api.Pointer(80),
			ForceHttps: false,
		}},
	}}
	require.Nil(f, m.Config.DisableMachineAutostart)
	require.Equal(f, want, m.Config.Services)

	f.Fly("machine update -a %s %s --autostart -y", appName, m.ID)
	m = f.MachinesList(appName)[0]
	want = []api.MachineService{{
		Protocol:     "tcp",
		InternalPort: 81,
		Autostart:    api.Pointer(true),
		Autostop:     api.Pointer(true),
		Ports: []api.MachinePort{{
			Port:       api.Pointer(80),
			ForceHttps: false,
		}},
	}}
	require.Nil(f, m.Config.DisableMachineAutostart)
	require.Equal(f, want, m.Config.Services)

	f.Fly("machine update -a %s %s --autostart=false --autostop=false -y", appName, m.ID)
	m = f.MachinesList(appName)[0]
	want = []api.MachineService{{
		Protocol:     "tcp",
		InternalPort: 81,
		Autostart:    api.Pointer(false),
		Autostop:     api.Pointer(false),
		Ports: []api.MachinePort{{
			Port:       api.Pointer(80),
			ForceHttps: false,
		}},
	}}
	require.Nil(f, m.Config.DisableMachineAutostart)
	require.Equal(f, want, m.Config.Services)
}

// test --standby-for
func TestFlyMachineRun_case02(t *testing.T) {
	f := testlib.NewTestEnvFromEnv(t)
	appName := f.CreateRandomAppMachines()

	findNewMachine := func(machineList []*api.Machine, knownIDs []string) *api.Machine {
		for _, m := range machineList {
			if !slices.Contains(knownIDs, m.ID) {
				return m
			}
		}
		return nil
	}

	f.Fly("machine run -a %s nginx", appName)
	ml := f.MachinesList(appName)
	require.Equal(f, 1, len(ml))
	og := ml[0]
	require.Empty(f, og.Config.Standbys)

	// Run a another machine and set it as standby of first
	f.Fly("machine run -a %s nginx --standby-for=%s", appName, og.ID)
	ml = f.MachinesList(appName)
	require.Equal(f, 2, len(ml))

	// Mahcine must be stopped and be standby for first machine ID
	s1 := findNewMachine(ml, []string{og.ID})
	require.Equal(f, "stopped", s1.State)
	require.Equal(f, []string{og.ID}, s1.Config.Standbys)

	// Clear the standbys field
	f.Fly("machine update -a %s %s --standby-for=''", appName, s1.ID)
	ml = f.MachinesList(appName)
	require.Equal(f, 2, len(ml))
	require.Equal(f, "started", s1.State)
	require.Empty(f, s1.Config.Standbys)

	// Clone and set its standby to the source
	f.Fly("machine clone -a %s %s --standby-for=source,%s", appName, og.ID, s1.ID)
	ml = f.MachinesList(appName)
	require.Equal(f, 3, len(ml))
	s2 := findNewMachine(ml, []string{og.ID, s1.ID})
	require.Equal(f, "stopped", s2.State)
	require.Equal(f, []string{og.ID, s1.ID}, s1.Config.Standbys)
}
