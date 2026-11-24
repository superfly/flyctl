//go:build integration
// +build integration

package preflight

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/test/preflight/testlib"
)

// test --port and --autostart --autostop flags
func TestFlyMachineRun_autoStartStop(t *testing.T) {
	f := testlib.NewTestEnvFromEnv(t)
	appName := f.CreateRandomAppMachines()

	f.Fly("machine run -a %s nginx --port 80:81 --autostop --region %s", appName, f.PrimaryRegion())
	ml := f.WaitForMachineCount(appName, 1)

	m := ml[0]
	requireServices := func(want []fly.MachineService) {
		f.WaitForMachines(appName, func(c assert.TestingT, machines []*fly.Machine) {
			if assert.Len(c, machines, 1) && assert.NotNil(c, machines[0].Config) {
				assert.Equal(c, m.ID, machines[0].ID)
				assert.Nil(c, machines[0].Config.DisableMachineAutostart)
				assert.Equal(c, want, machines[0].Config.Services)
			}
		})
	}
	want := []fly.MachineService{{
		Protocol:     "tcp",
		InternalPort: 81,
		Autostop:     fly.Pointer(fly.MachineAutostopStop),
		Ports: []fly.MachinePort{{
			Port:       fly.Pointer(80),
			ForceHTTPS: false,
		}},
	}}
	requireServices(want)

	// Only a rejected lease acquisition is safe to retry. Other update errors
	// can arrive after the mutation and must fail the test immediately.
	updateMachine := func(flags string) {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		for {
			res := f.FlyContextAndConfig(ctx, testlib.FlyCmdConfig{NoAssertSuccessfulExit: true}, "machine update -a %s %s %s -y", appName, m.ID, flags)
			if res.ExitCode() == 0 {
				return
			}
			stderr := res.StdErrString()
			if !strings.Contains(stderr, "Error: failed to obtain lease:") || !strings.Contains(stderr, "lease currently held") {
				res.AssertSuccessfulExit()
			}
			f.Logf("waiting for previous command's lease: %s", stderr)
			select {
			case <-ctx.Done():
				res.AssertSuccessfulExit()
			case <-time.After(2 * time.Second):
			}
		}
	}

	updateMachine("--autostart")
	want = []fly.MachineService{{
		Protocol:     "tcp",
		InternalPort: 81,
		Autostart:    fly.Pointer(true),
		Autostop:     fly.Pointer(fly.MachineAutostopStop),
		Ports: []fly.MachinePort{{
			Port:       fly.Pointer(80),
			ForceHTTPS: false,
		}},
	}}
	requireServices(want)

	updateMachine("--autostart=false --autostop=false")
	want = []fly.MachineService{{
		Protocol:     "tcp",
		InternalPort: 81,
		Autostart:    fly.Pointer(false),
		Autostop:     fly.Pointer(fly.MachineAutostopOff),
		Ports: []fly.MachinePort{{
			Port:       fly.Pointer(80),
			ForceHTTPS: false,
		}},
	}}
	requireServices(want)
}

// test that the deprecated --standby-for never creates a standby machine
func TestFlyMachineRun_standbyFor(t *testing.T) {
	f := testlib.NewTestEnvFromEnv(t)
	appName := f.CreateRandomAppMachines()

	findNewMachine := func(machineList []*fly.Machine, knownIDs []string) *fly.Machine {
		for _, m := range machineList {
			if !slices.Contains(knownIDs, m.ID) {
				return m
			}
		}
		return nil
	}
	findMachineByID := func(machineList []*fly.Machine, ID string) *fly.Machine {
		for _, m := range machineList {
			if m.ID == ID {
				return m
			}
		}
		return nil
	}

	f.Fly("machine run -a %s nginx --region %s", appName, f.PrimaryRegion())
	ml := f.WaitForMachineCount(appName, 1)
	og := ml[0]
	require.Empty(f, og.Config.Standbys)

	// Running with the flag creates a regular machine
	f.Fly("machine run -a %s nginx --standby-for=%s --region %s", appName, og.ID, f.PrimaryRegion())
	ml = f.WaitForMachineCount(appName, 2)
	m1 := findNewMachine(ml, []string{og.ID})
	require.Empty(f, m1.Config.Standbys)
	require.NotContains(f, m1.Config.Env, "FLY_STANDBY_FOR")

	// Updating with the flag doesn't turn a machine into a standby
	f.Fly("machine update -a %s %s --standby-for=%s -y", appName, m1.ID, og.ID)
	ml = f.WaitForMachineCount(appName, 2)
	m1 = findMachineByID(ml, m1.ID)
	require.Empty(f, m1.Config.Standbys)
	require.NotContains(f, m1.Config.Env, "FLY_STANDBY_FOR")

	// Cloning with the flag creates a regular machine
	f.Fly("machine clone -a %s %s --standby-for=source,%s", appName, og.ID, m1.ID)
	ml = f.WaitForMachineCount(appName, 3)
	m2 := findNewMachine(ml, []string{og.ID, m1.ID})
	require.Empty(f, m2.Config.Standbys)
	require.NotContains(f, m2.Config.Env, "FLY_STANDBY_FOR")
}

// test --rootfs-size flag
func TestFlyMachineRun_rootfsSize(t *testing.T) {
	f := testlib.NewTestEnvFromEnv(t)
	appName := f.CreateRandomAppMachines()

	// Run with --rootfs-size
	f.Fly("machine run -a %s nginx --rootfs-size 5 --region %s", appName, f.PrimaryRegion())
	ml := f.WaitForMachineCount(appName, 1)
	m := ml[0]
	require.NotNil(f, m.Config.Rootfs)
	require.Equal(f, uint64(5), m.Config.Rootfs.SizeGB)

	// Update with human-readable size
	f.Fly("machine update -a %s %s --rootfs-size 10gb -y", appName, m.ID)
	f.WaitForMachines(appName, func(c assert.TestingT, machines []*fly.Machine) {
		if assert.Len(c, machines, 1) && assert.NotNil(c, machines[0].Config) && assert.NotNil(c, machines[0].Config.Rootfs) {
			assert.Equal(c, m.ID, machines[0].ID)
			assert.Equal(c, uint64(10), machines[0].Config.Rootfs.SizeGB)
		}
	})
}

// test --port (add, update, remove services and ports)
func TestFlyMachineRun_port(t *testing.T) {
	f := testlib.NewTestEnvFromEnv(t)
	appName := f.CreateRandomAppMachines()

	// The machines list can lag a completed update for a moment, so retry
	// the read instead of asserting on the first response.
	requireServices := func(want []fly.MachineService) {
		require.EventuallyWithT(t, func(c *assert.CollectT) {
			ml := f.Collect(c).MachinesList(appName)
			if !assert.Len(c, ml, 1) {
				return
			}
			assert.Equal(c, want, ml[0].Config.Services)
		}, 30*time.Second, 2*time.Second, "machine services never matched the update")
	}

	f.Fly("machine run -a %s nginx --port 443:80/tcp:http:tls --region %s", appName, f.PrimaryRegion())
	ml := f.WaitForMachineCount(appName, 1)

	m := ml[0]
	want := []fly.MachineService{{
		Protocol:     "tcp",
		InternalPort: 80,
		Ports: []fly.MachinePort{{
			Port:     fly.Pointer(443),
			Handlers: []string{"http", "tls"},
		}},
	}}
	require.Equal(f, want, m.Config.Services)
	require.Contains(f, []string{"created", "started"}, m.State)

	f.Fly("machine update -a %s %s -y --port 80/tcp:http --port 1001/udp", appName, m.ID)
	want = []fly.MachineService{{
		Protocol:     "tcp",
		InternalPort: 80,
		Ports: []fly.MachinePort{{
			Port:     fly.Pointer(443),
			Handlers: []string{"http", "tls"},
		}, {
			Port:     fly.Pointer(80),
			Handlers: []string{"http"},
		}},
	}, {
		Protocol:     "udp",
		InternalPort: 1001,
		Ports: []fly.MachinePort{{
			Port: fly.Pointer(1001),
		}},
	}}
	requireServices(want)

	f.Fly("machine update -a %s %s -y --port 80/tcp:- --port 1001/udp:tls", appName, m.ID)
	want = []fly.MachineService{{
		Protocol:     "udp",
		InternalPort: 1001,
		Ports: []fly.MachinePort{{
			Port:     fly.Pointer(1001),
			Handlers: []string{"tls"},
		}},
	}}
	requireServices(want)
}
