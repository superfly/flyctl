//go:build integration
// +build integration

package preflight

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/test/preflight/testlib"
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
