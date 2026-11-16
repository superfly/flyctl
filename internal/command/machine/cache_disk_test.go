package machine

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/internal/cmdutil/preparers"
	"github.com/superfly/flyctl/internal/flag"
)

func TestCacheDiskFlags(t *testing.T) {
	for _, tc := range []struct {
		name    string
		initial *fly.MachineCacheDrive
		args    []string
		want    *fly.MachineCacheDrive
		wantErr string
	}{
		{name: "create mounted", args: []string{"--cache-disk-size=1gb", "--cache-disk-path=/cache"}, want: &fly.MachineCacheDrive{SizeMB: 1024, Path: "/cache"}},
		{name: "create raw", args: []string{"--cache-disk-size=1gb", "--cache-disk-path="}, want: &fly.MachineCacheDrive{SizeMB: 1024}},
		{name: "create requires path", args: []string{"--cache-disk-size=1gb"}, wantErr: "--cache-disk-path is required when adding a cache disk"},
		{name: "legacy alias", args: []string{"--cachedrive-size=1024", "--cache-disk-path="}, want: &fly.MachineCacheDrive{SizeMB: 1024}},
		{name: "legacy alias requires path", args: []string{"--cachedrive-size=1024"}, wantErr: "--cache-disk-path is required when adding a cache disk"},
		{name: "zero size disk requires path", initial: &fly.MachineCacheDrive{}, args: []string{"--cache-disk-size=1gb"}, wantErr: "--cache-disk-path is required when adding a cache disk"},
		{name: "preserve", initial: &fly.MachineCacheDrive{SizeMB: 1024, Path: "/cache"}, want: &fly.MachineCacheDrive{SizeMB: 1024, Path: "/cache"}},
		{name: "resize", initial: &fly.MachineCacheDrive{SizeMB: 1024, Path: "/cache"}, args: []string{"--cache-disk-size=2048"}, want: &fly.MachineCacheDrive{SizeMB: 2048, Path: "/cache"}},
		{name: "resize raw", initial: &fly.MachineCacheDrive{SizeMB: 1024}, args: []string{"--cache-disk-size=2048"}, want: &fly.MachineCacheDrive{SizeMB: 2048}},
		{name: "move mount", initial: &fly.MachineCacheDrive{SizeMB: 1024, Path: "/cache"}, args: []string{"--cache-disk-path=/new-cache"}, want: &fly.MachineCacheDrive{SizeMB: 1024, Path: "/new-cache"}},
		{name: "mount", initial: &fly.MachineCacheDrive{SizeMB: 1024}, args: []string{"--cache-disk-path=/cache"}, want: &fly.MachineCacheDrive{SizeMB: 1024, Path: "/cache"}},
		{name: "unmount", initial: &fly.MachineCacheDrive{SizeMB: 1024, Path: "/cache"}, args: []string{"--cache-disk-path="}, want: &fly.MachineCacheDrive{SizeMB: 1024}},
		{name: "disable", initial: &fly.MachineCacheDrive{SizeMB: 1024, Path: "/cache"}, args: []string{"--cache-disk-size=0"}},
		{name: "disable absent disk", args: []string{"--cache-disk-size=0"}},
		{name: "path without size", args: []string{"--cache-disk-path=/cache"}, wantErr: "positive cache disk size"},
		{name: "disable with path", args: []string{"--cache-disk-size=0", "--cache-disk-path=/cache"}, wantErr: "positive cache disk size"},
		{name: "negative", args: []string{"--cache-disk-size=-1"}, wantErr: "must not be negative"},
		{name: "invalid", args: []string{"--cache-disk-size=invalid"}, wantErr: "invalid cache disk size"},
		{name: "server validates limits", args: []string{"--cache-disk-size=100000", "--cache-disk-path=/cache"}, want: &fly.MachineCacheDrive{SizeMB: 100000, Path: "/cache"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmd := newUpdate()
			require.NoError(t, cmd.Flags().Parse(tc.args))
			ctx, err := preparers.ApplyAliases(flag.NewContext(t.Context(), cmd.Flags()))
			require.NoError(t, err)
			config := &fly.MachineConfig{CacheDrive: tc.initial}
			err = applyCacheDiskFlags(ctx, config)
			if tc.wantErr != "" {
				require.ErrorContains(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, config.CacheDrive)
		})
	}
	for _, cmd := range []*cobra.Command{newRun(), newCreate(), newUpdate()} {
		require.NotNil(t, cmd.Flags().Lookup("cache-disk-size"))
		require.NotNil(t, cmd.Flags().Lookup("cache-disk-path"))
		require.NotEmpty(t, cmd.Flags().Lookup("cachedrive-size").Deprecated)
	}
}
