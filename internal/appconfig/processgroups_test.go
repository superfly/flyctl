package appconfig

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProcessNames(t *testing.T) {
	testcases := []struct {
		name               string
		filepath           string
		config             *Config
		defaultProcessName string
		processNames       []string
		format             string
	}{
		{
			name:               "Test nil config",
			defaultProcessName: "app",
			processNames:       []string{"app"},
			format:             "['app']",
		},
		{
			name:               "Test empty config",
			config:             &Config{},
			defaultProcessName: "app",
			processNames:       []string{"app"},
			format:             "['app']",
		},
		{
			name:               "empty config with non standard default group name",
			config:             &Config{defaultGroupName: "foo"},
			defaultProcessName: "foo",
			processNames:       []string{"foo"},
			format:             "['foo']",
		},
		{
			name:               "Test one process named 'web'",
			filepath:           "./testdata/processes-one.toml",
			defaultProcessName: "web",
			processNames:       []string{"web"},
			format:             "['web']",
		},
		{
			name:               "Test multi processes returns first name in order",
			filepath:           "./testdata/processes-multi.toml",
			defaultProcessName: "bar",
			processNames:       []string{"bar", "foo", "zzz"},
			format:             "['bar', 'foo', 'zzz']",
		},
		{
			name:               "Test multi processes includes default name 'app'",
			filepath:           "./testdata/processes-multiwithapp.toml",
			defaultProcessName: "app",
			processNames:       []string{"aaa", "abc", "app", "ass", "bbb"},
			format:             "['aaa', 'abc', 'app', 'ass', 'bbb']",
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := tc.config
			if tc.filepath != "" {
				var err error
				cfg, err = LoadConfig(tc.filepath)
				require.NoError(t, err)
			}
			// Test unknown platform version
			assert.Equal(t, tc.defaultProcessName, cfg.DefaultProcessName())
			assert.Equal(t, tc.processNames, cfg.ProcessNames())
			assert.Equal(t, tc.format, cfg.FormatProcessNames())

			// XXX: Break here because SetPlatform calls crash on nil Config
			if cfg == nil {
				return
			}

			// Test for machines
			require.NoError(t, cfg.SetMachinesPlatform())
			assert.Equal(t, tc.defaultProcessName, cfg.DefaultProcessName())
			assert.Equal(t, tc.processNames, cfg.ProcessNames())
			assert.Equal(t, tc.format, cfg.FormatProcessNames())

			if cfg.RawDefinition == nil {
				return
			}

			// Test for detached
			require.NoError(t, cfg.SetDetachedPlatform())
			assert.Equal(t, tc.defaultProcessName, cfg.DefaultProcessName())
			assert.Equal(t, tc.processNames, cfg.ProcessNames())
			assert.Equal(t, tc.format, cfg.FormatProcessNames())

			// Test for nomad
			require.NoError(t, cfg.SetNomadPlatform())
			assert.Equal(t, tc.defaultProcessName, cfg.DefaultProcessName())
			assert.Equal(t, tc.processNames, cfg.ProcessNames())
			assert.Equal(t, tc.format, cfg.FormatProcessNames())
		})
	}
}
