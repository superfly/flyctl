package mpg

import (
	"context"
	"testing"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/superfly/flyctl/internal/flag/flagctx"
	"github.com/superfly/flyctl/iostreams"
)

func TestWarnIfV2FlagUsed(t *testing.T) {
	const want = "The '--v2' flag is deprecated and no longer has any effect."

	cases := []struct {
		name  string
		value string // empty means the flag is left unset
		warn  bool
	}{
		{name: "flag omitted", warn: false},
		{name: "explicit --v2=true", value: "true", warn: true},
		{name: "explicit --v2=false", value: "false", warn: true},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, _, stderr := iostreams.Test()
			ctx := iostreams.NewContext(context.Background(), ios)

			flagSet := pflag.NewFlagSet("test", pflag.ContinueOnError)
			flagSet.Bool("v2", true, "")
			if tt.value != "" {
				require.NoError(t, flagSet.Set("v2", tt.value))
			}
			ctx = flagctx.NewContext(ctx, flagSet)

			warnIfV2FlagUsed(ctx)

			if tt.warn {
				assert.Contains(t, stderr.String(), want)
			} else {
				assert.Empty(t, stderr.String())
			}
		})
	}
}
