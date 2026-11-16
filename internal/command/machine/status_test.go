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
