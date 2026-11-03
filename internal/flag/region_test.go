package flag

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// parseRegionFlag builds a command using the real Region flag definition and
// parses args against it, mirroring how commands register the flag.
func parseRegionFlag(t *testing.T, args ...string) error {
	t.Helper()

	cmd := &cobra.Command{Use: "test"}
	Region().addTo(cmd)

	return cmd.Flags().Parse(args)
}

// A repeated --region must not be silently accepted (last value wins); the user
// should get an error telling them the flag was set more than once.
func TestRegionFlagRejectsRepeatedFlag(t *testing.T) {
	err := parseRegionFlag(t, "--region", "sea", "--region", "iad")
	require.Error(t, err)
	assert.ErrorContains(t, err, "region")
	assert.ErrorContains(t, err, "set more than once")
}

// The -r shorthand previously swallowed an adjacent single-hyphen long flag
// (e.g. "-restart=no") as a region value of "estart=no".
func TestRegionFlagRejectsSingleHyphenLongFlagValue(t *testing.T) {
	err := parseRegionFlag(t, "-r", "-restart=no")
	require.Error(t, err)
	assert.ErrorContains(t, err, "region")
	assert.ErrorContains(t, err, `--restart`)
}

// A single, well formed region value must still parse.
func TestRegionFlagAcceptsSingleValue(t *testing.T) {
	require.NoError(t, parseRegionFlag(t, "--region", "sea"))
	require.NoError(t, parseRegionFlag(t, "-r", "sea"))
}
