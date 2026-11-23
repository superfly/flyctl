package machine

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/mock"
	"github.com/superfly/flyctl/iostreams"
)

func TestMachineStatusCacheDisk(t *testing.T) {
	for _, tc := range []struct {
		name, path string
		json       bool
	}{
		{name: "mounted", path: "/cache"},
		{name: "raw"},
		{name: "json", path: "/cache", json: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := &fly.Machine{ID: "abc", Config: &fly.MachineConfig{Guest: &fly.MachineGuest{}, CacheDrive: &fly.MachineCacheDrive{SizeMB: 1024, Path: tc.path}}}
			client := &mock.FlapsClient{GetFunc: func(context.Context, string, string) (*fly.Machine, error) { return m, nil }}
			cmd := newStatus()
			args := []string{m.ID}
			if tc.json {
				args = append(args, "--display-config")
			}
			require.NoError(t, cmd.Flags().Parse(args))
			ios, _, out, _ := iostreams.Test()
			ctx := iostreams.NewContext(t.Context(), ios)
			ctx = appconfig.WithName(ctx, "test")
			ctx = flapsutil.NewContextWithClient(ctx, client)
			ctx = flag.NewContext(ctx, cmd.Flags())
			require.NoError(t, runMachineStatus(ctx))
			if tc.json {
				var config map[string]json.RawMessage
				require.NoError(t, json.Unmarshal(out.Bytes(), &config))
				require.Contains(t, config, "cache_disk")
				require.NotContains(t, config, "cache_drive")
				require.JSONEq(t, `{"size_mb":1024,"path":"/cache"}`, string(config["cache_disk"]))
			} else {
				require.Contains(t, out.String(), "Cache Disk Size")
				require.Contains(t, out.String(), "1024 MiB")
				if tc.path == "" {
					require.Contains(t, out.String(), "raw device")
				} else {
					require.Contains(t, out.String(), tc.path)
				}
			}
		})
	}
}

func TestMachineStatusRootfsUsage(t *testing.T) {
	for _, tc := range []struct{ name, state, events, want string }{
		{"unmeasured", "started", `[]`, "unknown (measured at next stop/suspend)"},
		{"stopped", "stopped", `[{"status":"stopped","timestamp":1000,"request":{"rootfs_usage":{"image_bytes":"1048576","overlay_bytes":"1048576"}}}]`, "2.0 MiB"},
		{"suspended zero", "suspended", `[{"status":"suspended","timestamp":1000,"request":{"rootfs_usage":{"image_bytes":"0","overlay_bytes":"0"}}}]`, "0 B"},
		{"newest measurement", "started", `[{"status":"started","timestamp":3000},{"status":"suspended","timestamp":2000,"request":{"rootfs_usage":{"image_bytes":"1048576","overlay_bytes":"1048576"}}},{"status":"stopped","timestamp":1000,"request":{"rootfs_usage":{"image_bytes":"1048576","overlay_bytes":"0"}}}]`, "2.0 MiB (measured "},
		{"other status", "started", `[{"status":"started","timestamp":1000,"request":{"rootfs_usage":{"image_bytes":"1048576","overlay_bytes":"1048576"}}}]`, "2.0 MiB (measured "},
		{"first measurement wins", "started", `[{"timestamp":1000,"request":{"rootfs_usage":{"image_bytes":"1048576","overlay_bytes":"0"}}},{"timestamp":2000,"request":{"rootfs_usage":{"image_bytes":"1048576","overlay_bytes":"1048576"}}}]`, "1.0 MiB (measured "},
		{"no request", "started", `[{"status":"stopped","timestamp":1000}]`, "unknown (measured at next stop/suspend)"},
		{"partial", "started", `[{"status":"stopped","timestamp":1000,"request":{"rootfs_usage":{"image_bytes":"1048576"}}}]`, "unknown (measured at next stop/suspend)"},
		{"overflow", "started", `[{"status":"stopped","timestamp":1000,"request":{"rootfs_usage":{"image_bytes":"18446744073709551615","overlay_bytes":"1"}}}]`, "unknown (measured at next stop/suspend)"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := &fly.Machine{ID: "abc", State: tc.state, Config: &fly.MachineConfig{Guest: &fly.MachineGuest{}}}
			require.NoError(t, json.Unmarshal([]byte(tc.events), &m.Events))
			client := &mock.FlapsClient{GetFunc: func(context.Context, string, string) (*fly.Machine, error) { return m, nil }}
			cmd := newStatus()
			require.NoError(t, cmd.Flags().Parse([]string{m.ID}))
			ios, _, out, _ := iostreams.Test()
			ctx := iostreams.NewContext(t.Context(), ios)
			ctx = appconfig.WithName(ctx, "test")
			ctx = flapsutil.NewContextWithClient(ctx, client)
			ctx = flag.NewContext(ctx, cmd.Flags())
			require.NoError(t, runMachineStatus(ctx))
			require.Contains(t, out.String(), "RootFS Size")
			require.Contains(t, out.String(), tc.want)
			if tc.state == "stopped" || tc.state == "suspended" {
				require.NotContains(t, out.String(), "(measured ")
			}
		})
	}
}
