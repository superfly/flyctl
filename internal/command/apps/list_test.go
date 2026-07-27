package apps

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRenderEmptyApps(t *testing.T) {
	tests := []struct {
		name       string
		jsonOutput bool
		quiet      bool
		want       string
	}{
		{
			name: "default",
			want: "No apps found.\n",
		},
		{
			name:  "quiet",
			quiet: true,
			want:  "",
		},
		{
			name:       "JSON",
			jsonOutput: true,
			want:       "[]\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer

			require.NoError(t, renderEmptyApps(&out, tt.jsonOutput, tt.quiet))
			assert.Equal(t, tt.want, out.String())
		})
	}
}
