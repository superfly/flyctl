//go:build integration
// +build integration

package preflight

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/test/preflight/testlib"
)

func TestPostgres_singleNode(t *testing.T) {
	f := testlib.NewTestEnvFromEnv(t)
	appName := f.CreateRandomAppName()

	f.Fly(
		"pg create --org %s --name %s --region %s --initial-cluster-size 1 --vm-size shared-cpu-1x --volume-size 1",
		f.OrgSlug(), appName, f.PrimaryRegion(),
	)
	f.Fly("status -a %s", appName)
	f.Fly("config save -a %s", appName)
	flyToml := f.UnmarshalFlyToml()
	want := map[string]any{
		"app":            appName,
		"primary_region": f.PrimaryRegion(),
		"env": map[string]any{
			"PRIMARY_REGION": f.PrimaryRegion(),
		},
		"metrics": map[string]any{
			"port": int64(9187),
			"path": "/metrics",
		},
		"mounts": []map[string]any{{
			"source":      "pg_data",
			"destination": "/data",
		}},
		"services": []map[string]any{
			{
				"internal_port": int64(5432),
				"protocol":      "tcp",
				"concurrency": map[string]any{
					"type":       "connections",
					"soft_limit": int64(1000),
					"hard_limit": int64(1000),
				},
				"ports": []map[string]any{
					{"handlers": []any{"pg_tls"}, "port": int64(5432)},
				},
			},
			{
				"internal_port": int64(5433),
				"protocol":      "tcp",
				"concurrency": map[string]any{
					"type":       "connections",
					"soft_limit": int64(1000),
					"hard_limit": int64(1000),
				},
				"ports": []map[string]any{
					{"handlers": []any{"pg_tls"}, "port": int64(5433)},
				},
			},
		},
		"checks": map[string]any{
			"pg": map[string]any{
				"type":     "http",
				"port":     int64(5500),
				"path":     "/flycheck/pg",
				"interval": "15s",
				"timeout":  "10s",
			},
			"role": map[string]any{
				"type":     "http",
				"port":     int64(5500),
				"path":     "/flycheck/role",
				"interval": "15s",
				"timeout":  "10s",
			},
			"vm": map[string]any{
				"type":     "http",
				"port":     int64(5500),
				"path":     "/flycheck/vm",
				"interval": "15s",
				"timeout":  "10s",
			},
		},
	}
	require.Equal(f, want, flyToml)
}

func TestPostgres_autostart(t *testing.T) {
	f := testlib.NewTestEnvFromEnv(t)
	appName := f.CreateRandomAppName()

	f.Fly("pg create --org %s --name %s --region %s --initial-cluster-size 1 --vm-size shared-cpu-1x --volume-size 1", f.OrgSlug(), appName, f.PrimaryRegion())
	machList := f.MachinesList(appName)
	require.Equal(t, 1, len(machList), "expected exactly 1 machine after launch")
	firstMachine := machList[0]

	for _, svc := range firstMachine.Config.Services {
		require.NotNil(t, svc.Autostart, "autostart when not set defaults to True")
		require.False(t, *svc.Autostart, "autostart wasn't disabled")
	}

	appName = f.CreateRandomAppName()
	f.Fly("pg create --org %s --name %s --region %s --initial-cluster-size 1 --vm-size shared-cpu-1x --volume-size 1 --autostart", f.OrgSlug(), appName, f.PrimaryRegion())
	machList = f.MachinesList(appName)
	require.Equal(t, 1, len(machList), "expected exactly 1 machine after launch")
	firstMachine = machList[0]
	for _, svc := range firstMachine.Config.Services {
		// Autostart defaults to True if not set
		if svc.Autostart != nil {
			require.True(t, *svc.Autostart, "autostart wasn't disabled")
		}
	}
}

func TestPostgres_FlexFailover(t *testing.T) {
	f := testlib.NewTestEnvFromEnv(t)
	appName := f.CreateRandomAppName()
	findLeaderID := func(ml []*api.Machine) string {
		for _, mach := range ml {
			for _, chk := range mach.Checks {
				if chk.Name == "role" && chk.Output == "primary" {
					return mach.ID
				}
			}
		}
		return ""
	}

	f.Fly("pg create --flex --org %s --name %s --region %s --initial-cluster-size 3 --vm-size shared-cpu-1x --volume-size 1", f.OrgSlug(), appName, f.PrimaryRegion())
	machList := f.MachinesList(appName)
	leaderMachineID := findLeaderID(machList)
	if leaderMachineID == "" {
		f.Fatalf("Failed to find PG cluster leader")
	}

	f.Fly("pg failover -a %s", appName)
	machList = f.MachinesList(appName)
	newLeaderMachineID := findLeaderID(machList)
	require.NotEqual(t, newLeaderMachineID, leaderMachineID, "Failover failed! PG Leader didn't change!")
}

func TestPostgres_NoMachines(t *testing.T) {
	f := testlib.NewTestEnvFromEnv(t)
	appName := f.CreateRandomAppName()

	f.Fly("pg create --org %s --name %s --region %s --initial-cluster-size 1 --vm-size shared-cpu-1x --volume-size 1", f.OrgSlug(), appName, f.PrimaryRegion())
	machList := f.MachinesList(appName)
	require.Equal(t, 1, len(machList), "expected exactly 1 machine after launch")
	firstMachine := machList[0]
	f.Fly("m destroy %s -a %s --force", firstMachine.ID, appName)
	result := f.Fly("status -a %s", appName)

	require.Contains(f, result.StdOut().String(), "No machines are available on this app")
}

func TestPostgres_haConfigSave(t *testing.T) {
	f := testlib.NewTestEnvFromEnv(t)
	appName := f.CreateRandomAppName()

	f.Fly(
		"pg create --org %s --name %s --region %s --initial-cluster-size 3 --vm-size shared-cpu-1x --volume-size 1",
		f.OrgSlug(), appName, f.PrimaryRegion(),
	)
	f.Fly("status -a %s", appName)
	f.Fly("config save -a %s", appName)
	ml := f.MachinesList(appName)
	require.Equal(f, 3, len(ml))
	flyToml := f.UnmarshalFlyToml()
	require.Equal(f, "shared-cpu-1x", ml[0].Config.Guest.ToSize())
	want := map[string]any{
		"app":            appName,
		"primary_region": f.PrimaryRegion(),
		"env": map[string]any{
			"PRIMARY_REGION": f.PrimaryRegion(),
		},
		"metrics": map[string]any{
			"port": int64(9187),
			"path": "/metrics",
		},
		"mounts": []map[string]any{{
			"source":      "pg_data",
			"destination": "/data",
		}},
		"services": []map[string]any{
			{
				"internal_port": int64(5432),
				"protocol":      "tcp",
				"concurrency": map[string]any{
					"type":       "connections",
					"soft_limit": int64(1000),
					"hard_limit": int64(1000),
				},
				"ports": []map[string]any{
					{"handlers": []any{"pg_tls"}, "port": int64(5432)},
				},
			},
			{
				"internal_port": int64(5433),
				"protocol":      "tcp",
				"concurrency": map[string]any{
					"type":       "connections",
					"soft_limit": int64(1000),
					"hard_limit": int64(1000),
				},
				"ports": []map[string]any{
					{"handlers": []any{"pg_tls"}, "port": int64(5433)},
				},
			},
		},
		"checks": map[string]any{
			"pg": map[string]any{
				"type":     "http",
				"port":     int64(5500),
				"path":     "/flycheck/pg",
				"interval": "15s",
				"timeout":  "10s",
			},
			"role": map[string]any{
				"type":     "http",
				"port":     int64(5500),
				"path":     "/flycheck/role",
				"interval": "15s",
				"timeout":  "10s",
			},
			"vm": map[string]any{
				"type":     "http",
				"port":     int64(5500),
				"path":     "/flycheck/vm",
				"interval": "15s",
				"timeout":  "10s",
			},
		},
	}
	require.Equal(f, want, flyToml)
}
