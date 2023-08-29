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
func TestFlyMachineRun_autoStartStop(t *testing.T) {
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
			ForceHTTPS: false,
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
			ForceHTTPS: false,
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
			ForceHTTPS: false,
		}},
	}}
	require.Nil(f, m.Config.DisableMachineAutostart)
	require.Equal(f, want, m.Config.Services)
}

// test --standby-for
func TestFlyMachineRun_standbyFor(t *testing.T) {
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
	findMachineByID := func(machineList []*api.Machine, ID string) *api.Machine {
		for _, m := range machineList {
			if m.ID == ID {
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
	require.Contains(f, []string{"created", "stopped"}, s1.State)
	require.Equal(f, []string{og.ID}, s1.Config.Standbys)

	// Clear the standbys field
	f.Fly("machine update -a %s %s --standby-for='' -y", appName, s1.ID)
	ml = f.MachinesList(appName)
	s1 = findMachineByID(ml, s1.ID)
	require.Equal(f, 2, len(ml))
	// Updating a stopped machine doesn't start it
	require.Equal(f, "started", s1.State)
	require.Empty(f, s1.Config.Standbys)

	// Clone and set its standby to the source
	f.Fly("machine clone -a %s %s --standby-for=source,%s", appName, og.ID, s1.ID)
	ml = f.MachinesList(appName)
	require.Equal(f, 3, len(ml))
	s2 := findNewMachine(ml, []string{og.ID, s1.ID})
	require.Contains(f, []string{"created", "stopped"}, s2.State)
	require.Equal(f, []string{og.ID, s1.ID}, s2.Config.Standbys)

	// Finally update the standby list to only one machine
	f.Fly("machine update -a %s %s --standby-for=%s -y", appName, s2.ID, s1.ID)
	ml = f.MachinesList(appName)
	s2 = findMachineByID(ml, s2.ID)
	require.Equal(f, "stopped", s2.State)
	require.Equal(f, []string{s1.ID}, s2.Config.Standbys)
}

// test --port (add, update, remove services and ports)
func TestFlyMachineRun_port(t *testing.T) {
	f := testlib.NewTestEnvFromEnv(t)
	appName := f.CreateRandomAppMachines()

	f.Fly("machine run -a %s nginx --port 443:80/tcp:http:tls", appName)
	ml := f.MachinesList(appName)
	require.Equal(f, 1, len(ml))

	m := ml[0]
	want := []api.MachineService{{
		Protocol:     "tcp",
		InternalPort: 80,
		Ports: []api.MachinePort{{
			Port:     api.Pointer(443),
			Handlers: []string{"http", "tls"},
		}},
	}}
	require.Equal(f, want, m.Config.Services)
	require.Contains(f, []string{"created", "started"}, m.State)

	result := f.Fly("dig %s.internal -a %s --short", appName, appName)
	require.NotEmpty(f, result.StdOut().String())

	f.Fly("machine update -a %s %s -y --port 80/tcp:http --port 1001/udp", appName, m.ID)
	m = f.MachinesList(appName)[0]
	want = []api.MachineService{{
		Protocol:     "tcp",
		InternalPort: 80,
		Ports: []api.MachinePort{{
			Port:     api.Pointer(443),
			Handlers: []string{"http", "tls"},
		}, {
			Port:     api.Pointer(80),
			Handlers: []string{"http"},
		}},
	}, {
		Protocol:     "udp",
		InternalPort: 1001,
		Ports: []api.MachinePort{{
			Port: api.Pointer(1001),
		}},
	}}
	require.Equal(f, want, m.Config.Services)

	f.Fly("machine update -a %s %s -y --port 80/tcp:- --port 1001/udp:tls", appName, m.ID)
	m = f.MachinesList(appName)[0]
	want = []api.MachineService{{
		Protocol:     "udp",
		InternalPort: 1001,
		Ports: []api.MachinePort{{
			Port:     api.Pointer(1001),
			Handlers: []string{"tls"},
		}},
	}}
	require.Equal(f, want, m.Config.Services)
}
