package appconfig

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/internal/buildinfo"
)

func TestToMachineConfig(t *testing.T) {
	cfg, err := LoadConfig("./testdata/tomachine.toml")
	require.NoError(t, err)

	want := &fly.MachineConfig{
		Env: map[string]string{"FOO": "BAR", "PRIMARY_REGION": "mia", "FLY_PROCESS_GROUP": "app"},
		Services: []fly.MachineService{
			{
				Protocol:     "tcp",
				InternalPort: 8080,
				Ports: []fly.MachinePort{
					{Port: fly.Pointer(80), Handlers: []string{"http"}, ForceHTTPS: true},
					{Port: fly.Pointer(443), Handlers: []string{"http", "tls"}, ForceHTTPS: false},
				},
			},
		},
		Metadata: map[string]string{
			"fly_platform_version": "v2",
			"fly_process_group":    "app",
			"fly_flyctl_version":   buildinfo.Version().String(),
		},
		Metrics: &fly.MachineMetrics{Port: 9999, Path: "/metrics"},
		Statics: []*fly.Static{{GuestPath: "/guest/path", UrlPrefix: "/url/prefix", TigrisBucket: "example-bucket", IndexDocument: "index.html"}},
		Mounts:  []fly.MachineMount{{Name: "data", Path: "/data"}},
		Checks: map[string]fly.MachineCheck{
			"listening": {Port: fly.Pointer(8080), Type: fly.Pointer("tcp")},
			"status": {
				Port:     fly.Pointer(8080),
				Type:     fly.Pointer("http"),
				Interval: fly.MustParseDuration("10s"),
				Timeout:  fly.MustParseDuration("1s"),
				HTTPPath: fly.Pointer("/status"),
			},
		},
		StopConfig: &fly.StopConfig{
			Timeout: fly.MustParseDuration("10s"),
			Signal:  fly.Pointer("SIGTERM"),
		},
		Init: fly.MachineInit{
			SwapSizeMB: fly.Pointer(512),
		},
		Restart: &fly.MachineRestart{
			Policy: fly.MachineRestartPolicyAlways,
		},
		DNS: &fly.DNSConfig{Nameservers: []string{"1.2.3.4"}},
	}

	got, err := cfg.ToMachineConfig("", nil)
	assert.NoError(t, err)
	assert.Equal(t, want, got)

	// Update a machine config
	got, err = cfg.ToMachineConfig("", &fly.MachineConfig{
		Guest:       &fly.MachineGuest{CPUs: 3},
		Schedule:    "24/7",
		AutoDestroy: true,
		Restart:     &fly.MachineRestart{Policy: "always"},
		DNS:         &fly.DNSConfig{SkipRegistration: true},
		Env:         map[string]string{"removed": "by-update"},
		Mounts:      []fly.MachineMount{{Name: "removed", Path: "/by/update"}},
		Metadata:    map[string]string{"retain": "propagated"},
		Init:        fly.MachineInit{Cmd: []string{"removed", "by", "update"}},
	})
	assert.NoError(t, err)
	assert.Equal(t, want.Env, got.Env)
	assert.Equal(t, want.Services, got.Services)
	assert.Equal(t, want.Checks, got.Checks)
	assert.Equal(t, &fly.MachineGuest{CPUs: 3}, got.Guest)
	assert.Equal(t, "24/7", got.Schedule)
	assert.Equal(t, true, got.AutoDestroy)
	assert.Equal(t, &fly.MachineRestart{Policy: "always"}, got.Restart)
	assert.Equal(t, &fly.DNSConfig{SkipRegistration: true, Nameservers: []string{"1.2.3.4"}}, got.DNS)
	assert.Equal(t, "propagated", got.Metadata["retain"])
	assert.Empty(t, got.Init.Cmd)
}

func TestToMachineConfig_Experimental(t *testing.T) {
	cfg, err := LoadConfig("./testdata/tomachine-experimental.toml")
	require.NoError(t, err)

	got, err := cfg.ToMachineConfig("", nil)
	require.NoError(t, err)
	assert.Equal(t, fly.MachineInit{
		Cmd:        []string{"/call", "me"},
		Entrypoint: []string{"/IgoFirst"},
		Exec:       []string{"ignore", "others"},
	}, got.Init)

	cfg.Processes = map[string]string{"app": "/override experimental"}
	got, err = cfg.ToMachineConfig("", nil)
	require.NoError(t, err)
	assert.Equal(t, fly.MachineInit{
		Cmd:        []string{"/override", "experimental"},
		Entrypoint: []string{"/IgoFirst"},
		Exec:       []string{"ignore", "others"},
	}, got.Init)
}

func TestToMachineConfig_nullifyManagedFields(t *testing.T) {
	cfg := NewConfig()

	src := &fly.MachineConfig{
		Env: map[string]string{"FOO": "BAR", "PRIMARY_REGION": "mia"},
		Services: []fly.MachineService{
			{
				Protocol:     "tcp",
				InternalPort: 8080,
				Ports: []fly.MachinePort{
					{Port: fly.Pointer(80), Handlers: []string{"http"}, ForceHTTPS: true},
					{Port: fly.Pointer(443), Handlers: []string{"http", "tls"}, ForceHTTPS: false},
				},
			},
		},
		Metrics: &fly.MachineMetrics{Port: 9999, Path: "/metrics"},
		Statics: []*fly.Static{{GuestPath: "/guest/path", UrlPrefix: "/url/prefix", TigrisBucket: "example-bucket", IndexDocument: "index.html"}},
		Mounts:  []fly.MachineMount{{Name: "data", Path: "/data"}},
		Checks: map[string]fly.MachineCheck{
			"listening": {Port: fly.Pointer(8080), Type: fly.Pointer("tcp")},
			"status": {
				Port:     fly.Pointer(8080),
				Type:     fly.Pointer("http"),
				Interval: fly.MustParseDuration("10s"),
				Timeout:  fly.MustParseDuration("1s"),
				HTTPPath: fly.Pointer("/status"),
			},
		},
	}

	got, err := cfg.ToMachineConfig("", src)
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"FLY_PROCESS_GROUP": "app"}, got.Env)
	assert.Empty(t, got.Metrics)
	assert.Empty(t, got.Services)
	assert.Empty(t, got.Checks)
	assert.Empty(t, got.Mounts)
	assert.Empty(t, got.Statics)
}

func TestToReleaseMachineConfig(t *testing.T) {
	cfg, err := LoadConfig("./testdata/tomachine.toml")
	require.NoError(t, err)

	want := &fly.MachineConfig{
		Init: fly.MachineInit{
			Cmd:        []string{"migrate-db"},
			SwapSizeMB: fly.Pointer(512),
		},
		Env: map[string]string{"FOO": "BAR", "PRIMARY_REGION": "mia", "RELEASE_COMMAND": "1", "FLY_PROCESS_GROUP": "fly_app_release_command"},
		Metadata: map[string]string{
			"fly_platform_version": "v2",
			"fly_process_group":    "fly_app_release_command",
			"fly_flyctl_version":   buildinfo.Version().String(),
		},
		AutoDestroy: true,
		Restart:     &fly.MachineRestart{Policy: fly.MachineRestartPolicyNo},
		DNS:         &fly.DNSConfig{SkipRegistration: true},
		StopConfig: &fly.StopConfig{
			Timeout: fly.MustParseDuration("10s"),
			Signal:  fly.Pointer("SIGTERM"),
		},
		Guest: &fly.MachineGuest{
			CPUKind:  "performance",
			CPUs:     2,
			MemoryMB: 4096,
		},
	}

	got, err := cfg.ToReleaseMachineConfig()
	assert.NoError(t, err)
	assert.Equal(t, want, got)
}

func TestToTestMachineConfig(t *testing.T) {
	cfg, err := LoadConfig("./testdata/tomachine-machinechecks.toml")
	require.NoError(t, err)

	want := &fly.MachineConfig{
		Init: fly.MachineInit{
			Cmd:        []string{"curl", "https://fly.io"},
			SwapSizeMB: fly.Pointer(512),
			Entrypoint: []string{"/bin/sh"},
		},
		Image: "curlimages/curl",
		Env: map[string]string{
			"PRIMARY_REGION":      "mia",
			"FLY_TEST_COMMAND":    "1",
			"FLY_PROCESS_GROUP":   "fly_app_test_machine_command",
			"FLY_TEST_MACHINE_IP": "",
		},
		Metadata: map[string]string{
			"fly_platform_version": "v2",
			"fly_process_group":    "fly_app_test_machine_command",
			"fly_flyctl_version":   buildinfo.Version().String(),
		},
		AutoDestroy: true,
		Restart:     &fly.MachineRestart{Policy: fly.MachineRestartPolicyNo},
		DNS:         &fly.DNSConfig{SkipRegistration: true},
		StopConfig: &fly.StopConfig{
			Timeout: nil,
			Signal:  nil,
		},
	}

	check := cfg.HTTPService.MachineChecks[0]
	got, err := cfg.ToTestMachineConfig(check, nil)
	assert.NoError(t, err)
	assert.Equal(t, got, want)
}

func TestToTestMachineConfigWKillInfo(t *testing.T) {
	t.Parallel()
	cfg, err := LoadConfig("./testdata/tomachine-machinechecks.toml")
	require.NoError(t, err)

	cfg.KillSignal = fly.StringPointer("SIGABRT")
	cfg.KillTimeout = fly.MustParseDuration("60s")

	want := &fly.MachineConfig{
		Init: fly.MachineInit{
			Cmd:        []string{"curl", "https://fly.io"},
			SwapSizeMB: fly.Pointer(512),
			Entrypoint: []string{"/bin/sh"},
		},
		Image: "curlimages/curl",
		Env: map[string]string{
			"PRIMARY_REGION":      "mia",
			"FLY_TEST_COMMAND":    "1",
			"FLY_PROCESS_GROUP":   "fly_app_test_machine_command",
			"FLY_TEST_MACHINE_IP": "",
		},
		Metadata: map[string]string{
			"fly_platform_version": "v2",
			"fly_process_group":    "fly_app_test_machine_command",
			"fly_flyctl_version":   buildinfo.Version().String(),
		},
		AutoDestroy: true,
		Restart:     &fly.MachineRestart{Policy: fly.MachineRestartPolicyNo},
		DNS:         &fly.DNSConfig{SkipRegistration: true},
		StopConfig: &fly.StopConfig{
			Signal:  nil,
			Timeout: nil,
		},
	}

	check := cfg.HTTPService.MachineChecks[0]
	got, err := cfg.ToTestMachineConfig(check, nil)
	assert.NoError(t, err)
	assert.Equal(t, got, want)
}

func TestToTestMachineConfigWKillInfoAndOrigMachineKillInfo(t *testing.T) {
	t.Parallel()
	cfg, err := LoadConfig("./testdata/tomachine-machinechecks.toml")
	require.NoError(t, err)

	cfg.HTTPService.MachineChecks[0].KillSignal = fly.StringPointer("SIGTERM")
	cfg.HTTPService.MachineChecks[0].KillTimeout = fly.MustParseDuration("10s")

	want := &fly.MachineConfig{
		Init: fly.MachineInit{
			Cmd:        []string{"curl", "https://fly.io"},
			SwapSizeMB: fly.Pointer(512),
			Entrypoint: []string{"/bin/sh"},
		},
		Image: "curlimages/curl",
		Env: map[string]string{
			"PRIMARY_REGION":      "mia",
			"FLY_TEST_COMMAND":    "1",
			"FLY_PROCESS_GROUP":   "fly_app_test_machine_command",
			"FLY_TEST_MACHINE_IP": "",
		},
		Metadata: map[string]string{
			"fly_platform_version": "v2",
			"fly_process_group":    "fly_app_test_machine_command",
			"fly_flyctl_version":   buildinfo.Version().String(),
		},
		AutoDestroy: true,
		Restart:     &fly.MachineRestart{Policy: fly.MachineRestartPolicyNo},
		DNS:         &fly.DNSConfig{SkipRegistration: true},
		StopConfig: &fly.StopConfig{
			Signal:  fly.StringPointer("SIGTERM"),
			Timeout: fly.MustParseDuration("10s"),
		},
	}

	origMachine := &fly.Machine{
		Config: &fly.MachineConfig{
			StopConfig: &fly.StopConfig{
				Signal:  fly.StringPointer("SIGTERM"),
				Timeout: fly.MustParseDuration("10s"),
			},
		},
	}

	check := cfg.HTTPService.MachineChecks[0]
	got, err := cfg.ToTestMachineConfig(check, origMachine)
	assert.NoError(t, err)
	assert.Equal(t, got, want)
}

func TestToTestMachineConfigWKillInfoNoImageAndOrigMachineKillInfo(t *testing.T) {
	t.Parallel()
	cfg, err := LoadConfig("./testdata/tomachine-machinechecks.toml")
	require.NoError(t, err)

	cfg.HTTPService.MachineChecks[0].Image = ""
	cfg.HTTPService.MachineChecks[0].KillSignal = fly.StringPointer("SIGABRT")
	cfg.HTTPService.MachineChecks[0].KillTimeout = fly.MustParseDuration("30s")
	cfg.KillSignal = fly.StringPointer("SIGTERM")
	cfg.KillTimeout = fly.MustParseDuration("60s")

	want := &fly.MachineConfig{
		Init: fly.MachineInit{
			Cmd:        []string{"curl", "https://fly.io"},
			SwapSizeMB: fly.Pointer(512),
			Entrypoint: []string{"/bin/sh"},
		},
		Image: "nginx",
		Env: map[string]string{
			"PRIMARY_REGION":      "mia",
			"FLY_TEST_COMMAND":    "1",
			"FLY_PROCESS_GROUP":   "fly_app_test_machine_command",
			"FLY_TEST_MACHINE_IP": "",
		},
		Metadata: map[string]string{
			"fly_platform_version": "v2",
			"fly_process_group":    "fly_app_test_machine_command",
			"fly_flyctl_version":   buildinfo.Version().String(),
		},
		AutoDestroy: true,
		Restart:     &fly.MachineRestart{Policy: fly.MachineRestartPolicyNo},
		DNS:         &fly.DNSConfig{SkipRegistration: true},
		StopConfig: &fly.StopConfig{
			Signal:  fly.StringPointer("SIGABRT"),
			Timeout: fly.MustParseDuration("30s"),
		},
	}

	origMachine := &fly.Machine{
		HostStatus: fly.HostStatusOk,
		Config: &fly.MachineConfig{
			Image: "nginx",
			StopConfig: &fly.StopConfig{
				Signal:  fly.StringPointer("SIGTERM"),
				Timeout: fly.MustParseDuration("60s"),
			},
		},
	}

	check := cfg.HTTPService.MachineChecks[0]
	got, err := cfg.ToTestMachineConfig(check, origMachine)
	assert.NoError(t, err)
	assert.Equal(t, got, want)
}

func TestToTestMachineConfigNoImageAndOrigMachineKillInfo(t *testing.T) {
	t.Parallel()
	cfg, err := LoadConfig("./testdata/tomachine-machinechecks.toml")
	require.NoError(t, err)

	cfg.HTTPService.MachineChecks[0].Image = ""
	cfg.KillSignal = fly.StringPointer("SIGTERM")
	cfg.KillTimeout = fly.MustParseDuration("60s")

	want := &fly.MachineConfig{
		Init: fly.MachineInit{
			Cmd:        []string{"curl", "https://fly.io"},
			SwapSizeMB: fly.Pointer(512),
			Entrypoint: []string{"/bin/sh"},
		},
		Image: "nginx",
		Env: map[string]string{
			"PRIMARY_REGION":      "mia",
			"FLY_TEST_COMMAND":    "1",
			"FLY_PROCESS_GROUP":   "fly_app_test_machine_command",
			"FLY_TEST_MACHINE_IP": "",
		},
		Metadata: map[string]string{
			"fly_platform_version": "v2",
			"fly_process_group":    "fly_app_test_machine_command",
			"fly_flyctl_version":   buildinfo.Version().String(),
		},
		AutoDestroy: true,
		Restart:     &fly.MachineRestart{Policy: fly.MachineRestartPolicyNo},
		DNS:         &fly.DNSConfig{SkipRegistration: true},
		StopConfig: &fly.StopConfig{
			Signal:  fly.StringPointer("SIGTERM"),
			Timeout: fly.MustParseDuration("60s"),
		},
	}

	origMachine := &fly.Machine{
		HostStatus: fly.HostStatusOk,
		Config: &fly.MachineConfig{
			Image: "nginx",
			StopConfig: &fly.StopConfig{
				Signal:  fly.StringPointer("SIGTERM"),
				Timeout: fly.MustParseDuration("60s"),
			},
		},
	}

	check := cfg.HTTPService.MachineChecks[0]
	got, err := cfg.ToTestMachineConfig(check, origMachine)
	assert.NoError(t, err)
	assert.Equal(t, got, want)
}

func TestToTestMachineConfigWTestMachine(t *testing.T) {
	t.Parallel()
	cfg, err := LoadConfig("./testdata/tomachine.toml")
	require.NoError(t, err)

	want := &fly.MachineConfig{
		Init: fly.MachineInit{
			Cmd:        []string{"curl", "https://fly.io"},
			SwapSizeMB: fly.Pointer(512),
			Entrypoint: []string{"/bin/sh"},
		},
		Image: "curlimages/curl",
		Env: map[string]string{
			"PRIMARY_REGION":      "mia",
			"FLY_TEST_COMMAND":    "1",
			"FLY_PROCESS_GROUP":   "fly_app_test_machine_command",
			"FLY_TEST_MACHINE_IP": "1.2.3.4",
			"FOO":                 "BAR",
			"BAR":                 "BAZ",
		},
		Metadata: map[string]string{
			"fly_platform_version": "v2",
			"fly_process_group":    "fly_app_test_machine_command",
			"fly_flyctl_version":   buildinfo.Version().String(),
		},
		AutoDestroy: true,
		Restart:     &fly.MachineRestart{Policy: fly.MachineRestartPolicyNo},
		DNS:         &fly.DNSConfig{SkipRegistration: true},
		StopConfig: &fly.StopConfig{
			Timeout: nil,
			Signal:  nil,
		},
	}

	check := cfg.HTTPService.MachineChecks[0]
	machine := &fly.Machine{
		HostStatus: fly.HostStatusOk,
		ImageRef:   fly.MachineImageRef{},
		PrivateIP:  "1.2.3.4",
		Config: &fly.MachineConfig{
			Env: map[string]string{
				"BAR": "BAZ",
			},
			Init:  fly.MachineInit{Cmd: []string{"echo", "hello"}},
			Image: "nginx",
		},
	}
	got, err := cfg.ToTestMachineConfig(check, machine)
	assert.NoError(t, err)
	assert.Equal(t, want, got)
}

func TestToConsoleMachineConfig(t *testing.T) {
	cfg, err := LoadConfig("./testdata/tomachine.toml")
	require.NoError(t, err)

	want := &fly.MachineConfig{
		Init: fly.MachineInit{
			Exec:       []string{"/bin/sleep", "inf"},
			SwapSizeMB: fly.Pointer(512),
		},
		Env: map[string]string{
			"FOO":               "BAR",
			"PRIMARY_REGION":    "mia",
			"FLY_PROCESS_GROUP": "fly_app_console",
		},
		Metadata: map[string]string{
			"fly_platform_version": "v2",
			"fly_process_group":    "fly_app_console",
			"fly_flyctl_version":   buildinfo.Version().String(),
		},
		AutoDestroy: true,
		Restart: &fly.MachineRestart{
			Policy: fly.MachineRestartPolicyNo,
		},
		DNS: &fly.DNSConfig{SkipRegistration: true},
	}

	got, err := cfg.ToConsoleMachineConfig()
	assert.NoError(t, err)
	assert.Equal(t, want, got)
}

func TestToMachineConfig_multiProcessGroups(t *testing.T) {
	cfg, err := LoadConfig("./testdata/tomachine-processgroups.toml")
	require.NoError(t, err)

	testcases := []struct {
		name      string
		groupName string
		want      *fly.MachineConfig
	}{
		{
			name:      "default empty process group",
			groupName: "app",
			want: &fly.MachineConfig{
				Init: fly.MachineInit{Cmd: []string{"run-nginx"}},
				Services: []fly.MachineService{
					{
						Protocol:     "tcp",
						InternalPort: 8080,
						Ports: []fly.MachinePort{
							{Port: fly.Pointer(80), Handlers: []string{"http"}},
							{Port: fly.Pointer(443), Handlers: []string{"http", "tls"}},
						},
					},
					{Protocol: "tcp", InternalPort: 1111},
				},
				Checks: map[string]fly.MachineCheck{
					"listening": {Port: fly.Pointer(8080), Type: fly.Pointer("tcp")},
				},
			},
		},
		{
			name:      "vpn process group",
			groupName: "vpn",
			want: &fly.MachineConfig{
				Init: fly.MachineInit{Cmd: []string{"run-tailscale"}},
				Services: []fly.MachineService{
					{Protocol: "udp", InternalPort: 9999},
					{Protocol: "tcp", InternalPort: 1111},
				},
			},
		},
		{
			name:      "foo process group",
			groupName: "foo",
			want: &fly.MachineConfig{
				Init: fly.MachineInit{Cmd: []string{"keep", "me", "alive"}},
				Services: []fly.MachineService{
					{Protocol: "tcp", InternalPort: 1111},
				},
				Checks: map[string]fly.MachineCheck{
					"listening": {Port: fly.Pointer(8080), Type: fly.Pointer("tcp")},
				},
			},
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := cfg.ToMachineConfig(tc.groupName, nil)
			require.NoError(t, err)
			// We only care about fields that change for different process groups
			assert.Equal(t, tc.groupName, got.Metadata["fly_process_group"])
			assert.Equal(t, tc.want.Init, got.Init)
			assert.Equal(t, tc.want.Services, got.Services)
			assert.Equal(t, tc.want.Checks, got.Checks)
		})
	}
}

func TestToMachineConfig_defaultV2flytoml(t *testing.T) {
	cfg, err := LoadConfig("./testdata/tomachine-default-for-new-apps.toml")
	require.NoError(t, err)

	want := &fly.MachineConfig{
		Env: map[string]string{"PRIMARY_REGION": "ord", "FLY_PROCESS_GROUP": "app"},
		Services: []fly.MachineService{
			{
				Protocol:     "tcp",
				InternalPort: 8080,
				Ports: []fly.MachinePort{
					{Port: fly.Pointer(80), Handlers: []string{"http"}, ForceHTTPS: true},
					{Port: fly.Pointer(443), Handlers: []string{"http", "tls"}, ForceHTTPS: false},
				},
			},
		},
		Metadata: map[string]string{
			"fly_platform_version": "v2",
			"fly_process_group":    "app",
			"fly_flyctl_version":   buildinfo.Version().String(),
		},
		Checks: map[string]fly.MachineCheck{
			"alive": {
				Port:        fly.Pointer(8080),
				Type:        fly.Pointer("tcp"),
				Interval:    fly.MustParseDuration("15s"),
				Timeout:     fly.MustParseDuration("2s"),
				GracePeriod: fly.MustParseDuration("5s"),
			},
		},
	}

	got, err := cfg.ToMachineConfig("", nil)
	assert.NoError(t, err)
	assert.Equal(t, want, got)

	// A toplevel check without internal port must fail if there is no http service to relate to
	cfg.HTTPService = nil
	got, err = cfg.ToMachineConfig("", nil)
	assert.Nil(t, got)
	assert.ErrorContains(t, err, "has no port set")
}

func TestToReleaseMachineConfig_processGroupsAndMounts(t *testing.T) {
	cfg, err := LoadConfig("./testdata/tomachine-mounts.toml")
	require.NoError(t, err)

	got, err := cfg.ToMachineConfig("", nil)
	require.NoError(t, err)
	assert.Equal(t, []fly.MachineMount{{Name: "data", Path: "/data"}}, got.Mounts)

	got, err = cfg.ToMachineConfig("app", nil)
	require.NoError(t, err)
	assert.Equal(t, []fly.MachineMount{{Name: "data", Path: "/data"}}, got.Mounts)

	got, err = cfg.ToMachineConfig("back", nil)
	require.NoError(t, err)
	assert.Equal(t, []fly.MachineMount{{Name: "trash", Path: "/trash"}}, got.Mounts)

	got, err = cfg.ToMachineConfig("hola", nil)
	require.NoError(t, err)
	assert.Empty(t, got.Mounts)
}

func TestToMachineConfig_services(t *testing.T) {
	cfg, err := LoadConfig("./testdata/tomachine-services.toml")
	require.NoError(t, err)

	want := []fly.MachineService{
		{
			Protocol:     "tcp",
			InternalPort: 8080,
			Autostart:    fly.Pointer(true),
			Autostop:     fly.Pointer(fly.MachineAutostopStop),
			Ports: []fly.MachinePort{
				{Port: fly.Pointer(80), Handlers: []string{"http"}, ForceHTTPS: true},
				{Port: fly.Pointer(443), Handlers: []string{"http", "tls"}, ForceHTTPS: false},
			},
		},
		{
			Protocol:     "tcp",
			InternalPort: 1000,
			Autostart:    fly.Pointer(true),
			Autostop:     fly.Pointer(fly.MachineAutostopStop),
		},
		{
			Protocol:     "tcp",
			InternalPort: 1001,
			Autostart:    fly.Pointer(false),
			Autostop:     fly.Pointer(fly.MachineAutostopOff),
		},
		{
			Protocol:     "tcp",
			InternalPort: 1002,
			Autostart:    fly.Pointer(false),
		},
		{
			Protocol:     "tcp",
			InternalPort: 1003,
			Autostop:     fly.Pointer(fly.MachineAutostopStop),
		},
		{
			Protocol:     "tcp",
			InternalPort: 1004,
			Autostart:    nil,
			Autostop:     nil,
		},
	}

	got, err := cfg.ToMachineConfig("", nil)
	assert.NoError(t, err)
	assert.Equal(t, want, got.Services)
}

func TestToMachineConfig_compute(t *testing.T) {
	cfg, err := LoadConfig("./testdata/tomachine-compute.toml")
	require.NoError(t, err)

	testcases := []struct {
		name      string
		groupName string
		want      *fly.MachineGuest
	}{
		{
			name:      "app gets compute without processes set",
			groupName: "app",
			want: &fly.MachineGuest{
				CPUKind:  "shared",
				CPUs:     2,
				MemoryMB: 512,
			},
		},
		{
			name:      "worker gets compute without processes set",
			groupName: "worker",
			want: &fly.MachineGuest{
				CPUKind:  "shared",
				CPUs:     2,
				MemoryMB: 512,
			},
		},
		{
			name:      "whisper gets gpu and performance-8x",
			groupName: "whisper",
			want: &fly.MachineGuest{
				CPUKind:  "performance",
				CPUs:     8,
				MemoryMB: 65536,
				GPUKind:  "a100-pcie-40gb",
			},
		},
		{
			name:      "isolated gets dedication id and shared-cpu-1x",
			groupName: "isolated",
			want: &fly.MachineGuest{
				CPUKind:          "shared",
				CPUs:             1,
				MemoryMB:         256,
				HostDedicationID: "lookma-iamsolo",
			},
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := cfg.ToMachineConfig(tc.groupName, nil)
			require.NoError(t, err)
			assert.Equal(t, tc.want, got.Guest)
		})
	}
}

func TestToMachineConfig_compute_none(t *testing.T) {
	cfg, err := LoadConfig("./testdata/tomachine-compute-nodefault.toml")
	require.NoError(t, err)

	testcases := []struct {
		name      string
		groupName string
		want      *fly.MachineGuest
	}{
		{
			name:      "app group has no default guest set",
			groupName: "app",
			want:      nil,
		},
		{
			name:      "woo group has no default guest set",
			groupName: "woo",
			want:      nil,
		},
		{
			name:      "bar gets performance-4x",
			groupName: "bar",
			want: &fly.MachineGuest{
				CPUKind:  "performance",
				CPUs:     4,
				MemoryMB: 8192,
			},
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := cfg.ToMachineConfig(tc.groupName, nil)
			require.NoError(t, err)
			assert.Equal(t, tc.want, got.Guest)
		})
	}
}

func TestToMachineConfig_hostdedicationid(t *testing.T) {
	cfg, err := LoadConfig("./testdata/tomachine-hostdedicationid.toml")
	require.NoError(t, err)

	testcases := []struct {
		name        string
		groupName   string
		wantTopHDID string
		wantGuest   *fly.MachineGuest
	}{
		{
			name:        "toplevel hdid must prevail",
			groupName:   "front",
			wantTopHDID: "toplevel",
			wantGuest:   nil,
		},
		{
			name:        "back has hdid set as compute section",
			groupName:   "back",
			wantTopHDID: "specific",
			wantGuest: &fly.MachineGuest{
				CPUKind:          "shared",
				CPUs:             1,
				MemoryMB:         256,
				HostDedicationID: "specific",
			},
		},
		{
			name:        "other has not hdid set as compute section",
			groupName:   "other",
			wantTopHDID: "toplevel",
			wantGuest: &fly.MachineGuest{
				CPUKind:          "shared",
				CPUs:             4,
				MemoryMB:         1024,
				HostDedicationID: "toplevel",
			},
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			flatCfg, err := cfg.Flatten(tc.groupName)
			require.NoError(t, err)
			assert.Equal(t, tc.wantTopHDID, flatCfg.HostDedicationID)

			got, err := cfg.ToMachineConfig(tc.groupName, nil)
			require.NoError(t, err)
			assert.Equal(t, tc.wantGuest, got.Guest)
		})
	}
}
