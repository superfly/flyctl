package secrets

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/mock"
)

func TestRunImportRejectsMalformedInputBeforeUpdatingSecrets(t *testing.T) {
	for _, tt := range []struct {
		name  string
		input string
	}{
		{name: "unfinished multiline", input: "GOOD=value\nKEY=\"\"\"unfinished"},
		{name: "oversized line", input: "GOOD=value\nKEY=" + strings.Repeat("x", 1<<20)},
		{name: "invalid assignment", input: "GOOD=value\ninvalid-line"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			// Stdin is process-global, so these subtests must stay sequential.
			stdin, err := os.CreateTemp(t.TempDir(), "secrets-stdin")
			require.NoError(t, err)
			t.Cleanup(func() { _ = stdin.Close() })
			_, err = io.WriteString(stdin, tt.input)
			require.NoError(t, err)
			_, err = stdin.Seek(0, io.SeekStart)
			require.NoError(t, err)
			originalStdin := os.Stdin
			os.Stdin = stdin
			t.Cleanup(func() { os.Stdin = originalStdin })

			updates := 0
			client := &mock.FlapsClient{
				GetAppFunc: func(context.Context, string) (*flaps.App, error) {
					return &flaps.App{Name: "parser-test"}, nil
				},
				UpdateAppSecretsFunc: func(context.Context, string, map[string]*string) (*fly.UpdateAppSecretsResp, error) {
					updates++

					return nil, errors.New("unexpected secret update")
				},
			}
			ctx := appconfig.WithName(context.Background(), "parser-test")
			ctx = flapsutil.NewContextWithClient(ctx, client)
			flags := pflag.NewFlagSet("import", pflag.ContinueOnError)
			flags.Bool("stage", false, "")
			flags.Bool("detach", false, "")
			flags.Bool("dns-checks", false, "")
			ctx = flag.NewContext(ctx, flags)

			err = runImport(ctx)
			require.ErrorContains(t, err, "Failed to parse secrets from stdin")
			assert.Zero(t, updates)
		})
	}
}
