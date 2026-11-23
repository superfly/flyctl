package flag

import (
	"testing"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWithDisplayNoOptDefVals(t *testing.T) {
	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	fs.String("db", "", "database")
	fs.String("other", "", "other")
	db := fs.Lookup("db")
	db.NoOptDefVal = "sentinel"
	fs.Lookup("other").NoOptDefVal = "x"
	SetDisplayNoOptDefVal(db, "true")

	var usage string
	WithDisplayNoOptDefVals(fs, func() { usage = fs.FlagUsages() })
	assert.Contains(t, usage, `--db string[="true"]`)
	assert.Contains(t, usage, `--other string[="x"]`)
	assert.NotContains(t, usage, "sentinel")

	// Restored for parsing, also when render panics.
	assert.Equal(t, "sentinel", db.NoOptDefVal)
	assert.Panics(t, func() { WithDisplayNoOptDefVals(fs, func() { panic("boom") }) })
	assert.Equal(t, "sentinel", db.NoOptDefVal)

	require.NoError(t, fs.Parse([]string{"--db"}))
	assert.Equal(t, "sentinel", db.Value.String())
}
