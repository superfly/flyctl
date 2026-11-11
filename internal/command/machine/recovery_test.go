package machine

import (
	"context"
	"errors"
	"testing"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/require"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/prompt"
	"github.com/superfly/flyctl/iostreams"
)

func TestMachineSelectionRecovery(t *testing.T) {
	for _, tc := range []struct {
		name, app, want string
		selectFlag, id  bool
		headless        bool
	}{
		{name: "selection requires app", selectFlag: true, want: "specified with --app"},
		{name: "missing target", want: "a machine ID argument or an app name with --app"},
		{name: "headless discovery", app: "app", want: "fly machine list --app app`", headless: true},
		{name: "explicit target", app: "app", id: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			flags := pflag.NewFlagSet("selection", pflag.ContinueOnError)
			flags.Bool("select", tc.selectFlag, "")
			ios, _, _, _ := iostreams.Test()
			ctx := flag.NewContext(iostreams.NewContext(context.Background(), ios), flags)
			ctx = appconfig.WithName(ctx, tc.app)
			err := checkSelectConditions(ctx, tc.id)
			if tc.want == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, tc.want)
			}
			require.Equal(t, tc.headless, errors.Is(err, prompt.ErrNonInteractive))
		})
	}
}

func TestInvalidRestartNamesAllowedPolicies(t *testing.T) {
	cmd := newRun()
	require.NoError(t, cmd.Flags().Parse([]string{"--restart=invalid"}))
	ios, _, _, _ := iostreams.Test()
	ctx := flag.NewContext(iostreams.NewContext(context.Background(), ios), cmd.Flags())
	_, err := determineMachineConfig(ctx, &determineMachineConfigInput{})
	require.EqualError(t, err, "invalid --restart value \"invalid\"; use 'no', 'on-failure', or 'always'")
}
