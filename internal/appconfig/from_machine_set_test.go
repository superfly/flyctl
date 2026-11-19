package appconfig

import (
	"testing"

	"github.com/stretchr/testify/require"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/internal/machine"
	"github.com/superfly/flyctl/iostreams"
)

func TestQuotePosixWords(t *testing.T) {
	type test struct {
		input    []string
		expected []string
	}
	tests := []test{
		{input: []string{
			"nginx", "-g", "daemon off;",
		}, expected: []string{
			"nginx", "-g", "'daemon off;'",
		}},
		{input: []string{
			"",
		}, expected: []string{
			"",
		}},
		{input: []string{
			"/app",
		}, expected: []string{
			"/app",
		}},
		{input: []string{
			"bundle", "exec", "rake", "db:migrate",
		}, expected: []string{
			"bundle", "exec", "rake", "db:migrate",
		}},
		{input: []string{
			"/release_cmd.sh", "foo", "bar", "baz", "123", "--six=seven",
		}, expected: []string{
			"/release_cmd.sh", "foo", "bar", "baz", "123", "'--six=seven'",
		}},
		{input: []string{
			"echo", "hi there",
		}, expected: []string{
			"echo", `"hi there"`,
		}},
	}
	for _, tc := range tests {
		result := quotePosixWords(tc.input)
		require.EqualValues(t, tc.expected, result)
	}
}

func TestCacheDiskFromMachine(t *testing.T) {
	for _, tc := range []struct {
		name    string
		disk    *fly.MachineCacheDrive
		want    []CacheDisk
		warning bool
	}{
		{"mounted", &fly.MachineCacheDrive{SizeMB: 1024, Path: "/cache"}, []CacheDisk{{Size: "1024mib", Path: "/cache"}}, false},
		{"raw", &fly.MachineCacheDrive{SizeMB: 1024}, nil, true},
		{"disabled", &fly.MachineCacheDrive{}, nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ios, _, _, _ := iostreams.Test()
			m := machine.NewLeasableMachine(nil, ios, "test", &fly.Machine{ID: "abc", Config: &fly.MachineConfig{CacheDrive: tc.disk}}, false)
			cfg, warning := fromAppAndOneMachine(t.Context(), "test", m, &processGroupInfo{})
			require.Equal(t, tc.want, cfg.CacheDisks)
			if tc.warning {
				require.Contains(t, warning, "raw cache disk")
				require.Contains(t, warning, "will remove it")
			} else {
				require.Empty(t, warning)
			}
		})
	}
}
