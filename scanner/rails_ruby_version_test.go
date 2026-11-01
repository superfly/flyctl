package scanner

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseRubyVersionFile(t *testing.T) {
	cases := map[string]string{
		"3.3.0\n":            "3.3.0",
		"3.3.0":              "3.3.0",
		"ruby-3.2.2\n":       "3.2.2",
		"  3.4.1  \n":        "3.4.1",
		"3.3.0-p0\n":         "3.3.0",
		"# comment\n3.3.6\n": "3.3.6",
		"jruby-9.4.5.0\n":    "",
		"3.3\n":              "",
		"":                   "",
	}
	for content, want := range cases {
		t.Run(content, func(t *testing.T) {
			assert.Equal(t, want, parseRubyVersionFile(content))
		})
	}
}

func TestConfigureRailsReadsPlainRubyVersionFile(t *testing.T) {
	for _, content := range []string{"3.3.0\n", "ruby-3.3.0\n"} {
		t.Run(content, func(t *testing.T) {
			dir := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(dir, "Gemfile"), []byte("source 'https://rubygems.org'\ngem 'rails'\n"), 0o644))
			require.NoError(t, os.WriteFile(filepath.Join(dir, "config.ru"), []byte("run Rails.application\n"), 0o644))
			require.NoError(t, os.WriteFile(filepath.Join(dir, ".ruby-version"), []byte(content), 0o644))
			// with a Dockerfile the scanner doesn't need ruby or bundle on PATH
			require.NoError(t, os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte("FROM ruby:3.3.0\nEXPOSE 3000\n"), 0o644))
			t.Chdir(dir)

			si, err := configureRails(dir, &ScannerConfig{SkipHealthcheck: true})
			drainHealthcheckChannel()

			require.NoError(t, err)
			require.NotNil(t, si)
			assert.Equal(t, "ruby", si.Runtime.Language)
			assert.Equal(t, "3.3.0", si.Runtime.Version)
		})
	}
}
