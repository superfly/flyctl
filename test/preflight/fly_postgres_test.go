//go:build integration
// +build integration

package preflight

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/internal/appconfig"
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
	var (
		err     error
		f       = testlib.NewTestEnvFromEnv(t)
		appName = f.CreateRandomAppName()
	)

	f.Fly("pg create --org %s --name %s --region %s --initial-cluster-size 1 --vm-size shared-cpu-1x --volume-size 1", f.OrgSlug(), appName, f.PrimaryRegion())

	var machList []map[string]any

	result := f.Fly("m list --json -a %s", appName)
	err = json.Unmarshal(result.StdOut().Bytes(), &machList)
	if err != nil {
		f.Fatalf("failed to parse json: %v [output]: %s\n", err, result.StdOut().String())
	}
	require.Equal(t, 1, len(machList), "expected exactly 1 machine after launch")
	firstMachine := machList[0]

	config := firstMachine["config"].(map[string]interface{})
	if autostart_disabled, ok := config["disable_machine_autostart"]; ok {
		require.Equal(t, true, autostart_disabled.(bool), "autostart was enabled")
	} else {
		f.Fatalf("autostart wasn't disabled")
	}

	appName = f.CreateRandomAppName()

	f.Fly("pg create --org %s --name %s --region %s --initial-cluster-size 1 --vm-size shared-cpu-1x --volume-size 1 --autostart", f.OrgSlug(), appName, f.PrimaryRegion())

	result = f.Fly("m list --json -a %s", appName)
	err = json.Unmarshal(result.StdOut().Bytes(), &machList)
	if err != nil {
		f.Fatalf("failed to parse json: %v [output]: %s\n", err, result.StdOut().String())
	}
	require.Equal(t, 1, len(machList), "expected exactly 1 machine after launch")
	firstMachine = machList[0]

	config = firstMachine["config"].(map[string]interface{})

	if autostart_disabled, ok := config["disable_machine_autostart"]; ok {
		require.Equal(t, false, autostart_disabled.(bool), "autostart was enabled")
	}
}

func TestPostgres_FlexFailover(t *testing.T) {
	var (
		err     error
		f       = testlib.NewTestEnvFromEnv(t)
		appName = f.CreateRandomAppName()
	)

	f.Fly("pg create --flex --org %s --name %s --region %s --initial-cluster-size 3 --vm-size shared-cpu-1x --volume-size 1", f.OrgSlug(), appName, f.PrimaryRegion())

	result := f.Fly("m list --json -a %s", appName)

	var machList []map[string]any

	err = json.Unmarshal(result.StdOut().Bytes(), &machList)
	if err != nil {
		f.Fatalf("failed to parse json: %v [output]: %s\n", err, result.StdOut().String())
	}

	leaderMachineID := ""
	for _, mach := range machList {
		role := "unknown"

		checks := mach["checks"].([]interface{})
		for _, check := range checks {
			check := check.(map[string]interface{})
			if check["name"].(string) == "role" {
				role = check["output"].(string)
				break
			}
		}
		if role == "primary" {
			leaderMachineID = mach["id"].(string)
		}
	}
	if leaderMachineID == "" {
		f.Fatalf("Failed to find PG cluster leader")
	}

	f.Fly("pg failover -a %s", appName)

	result = f.Fly("m list --json -a %s", appName)
	err = json.Unmarshal(result.StdOut().Bytes(), &machList)
	if err != nil {
		f.Fatalf("failed to parse json: %v [output]: %s\n", err, result.StdOut().String())
	}

	newLeaderMachineID := ""

	for _, mach := range machList {
		role := "unknown"

		checks := mach["checks"].([]interface{})
		for _, check := range checks {
			check := check.(map[string]interface{})
			if check["name"].(string) == "role" {
				role = check["output"].(string)
				break
			}
		}
		if role == "primary" {
			newLeaderMachineID = mach["id"].(string)
		}
	}
	if newLeaderMachineID == "" {
		f.Fatalf("Failed to find PG cluster leader")
	}

	fmt.Println(newLeaderMachineID)
	fmt.Println(leaderMachineID)
	require.NotEqual(t, newLeaderMachineID, leaderMachineID, "Failover failed! PG Leader didn't change!")
}

func TestPostgres_NoMachines(t *testing.T) {
	var (
		err     error
		f       = testlib.NewTestEnvFromEnv(t)
		appName = f.CreateRandomAppName()
	)

	f.Fly("pg create --org %s --name %s --region %s --initial-cluster-size 1 --vm-size shared-cpu-1x --volume-size 1", f.OrgSlug(), appName, f.PrimaryRegion())

	// Remove the app's only machine, then run `status`
	result := f.Fly("m list --json -a %s", appName)
	var machList []map[string]any
	err = json.Unmarshal(result.StdOut().Bytes(), &machList)
	if err != nil {
		f.Fatalf("failed to parse json: %v [output]: %s\n", err, result.StdOut().String())
	}
	require.Equal(t, 1, len(machList), "expected exactly 1 machine after launch")
	firstMachine := machList[0]
	firstMachineId, ok := firstMachine["id"].(string)
	if !ok {
		f.Fatalf("could find or convert id key to string from %s, stdout: %s firstMachine: %v", result.CmdString(), result.StdOut().String(), firstMachine)
	}

	f.Fly("m destroy %s -a %s --force", firstMachineId, appName)
	result = f.Fly("status -a %s", appName)

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
