package prompt

import (
	"context"
	"os"
	"sync"
	"testing"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/require"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/iostreams"
)

func TestHeadlessPromptQuestions(t *testing.T) {
	for name, call := range map[string]func(context.Context) error{
		"string": func(ctx context.Context) error { var v string; return String(ctx, &v, "Question", "", true) },
		"string with help": func(ctx context.Context) error {
			var v string
			return StringWithHelp(ctx, &v, "Question", "", "help", true)
		},
		"integer":           func(ctx context.Context) error { var v int; return Int(ctx, &v, "Question", 0, true) },
		"password":          func(ctx context.Context) error { var v string; return Password(ctx, &v, "Question", true) },
		"select":            func(ctx context.Context) error { var v int; return Select(ctx, &v, "Question", "", "one") },
		"multi select":      func(ctx context.Context) error { var v []int; return MultiSelect(ctx, &v, "Question", nil, "one") },
		"confirm":           func(ctx context.Context) error { _, err := Confirm(ctx, "Question"); return err },
		"confirm yes":       func(ctx context.Context) error { _, err := ConfirmYes(ctx, "Question"); return err },
		"confirm formatted": func(ctx context.Context) error { _, err := Confirmf(ctx, "%s", "Question"); return err },
		"overwrite":         func(ctx context.Context) error { _, err := ConfirmOverwrite(ctx, "Question"); return err },
	} {
		t.Run(name, func(t *testing.T) {
			ios, _, out, errOut := iostreams.Test()
			err := call(iostreams.NewContext(context.Background(), ios))
			require.ErrorIs(t, err, ErrNonInteractive)
			require.ErrorContains(t, err, "Question")
			require.Empty(t, out.String())
			require.Empty(t, errOut.String())
		})
	}
}

func TestSurveyIORequiresTerminalFiles(t *testing.T) {
	file, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
	require.NoError(t, err)
	defer file.Close()
	for _, tc := range []struct {
		name    string
		inFile  bool
		outFile bool
		wantErr bool
	}{
		{"non-file input", false, true, true},
		{"non-file output", true, false, true},
		{"terminal files", true, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ios, _, _, _ := iostreams.Test()
			ios.SetStdinTTY(true)
			ios.SetStdoutTTY(true)
			if tc.inFile {
				ios.In = file
			}
			if tc.outFile {
				ios.Out = file
			}
			opt, err := newSurveyIO(iostreams.NewContext(context.Background(), ios), "Question")
			if tc.wantErr {
				require.ErrorIs(t, err, ErrNonInteractive)
				require.ErrorContains(t, err, "Question")
				require.Nil(t, opt)
			} else {
				require.NoError(t, err)
				require.NotNil(t, opt)
			}
		})
	}
}

type recoveryRegionsClient struct{ flapsutil.FlapsClient }

func (recoveryRegionsClient) GetRegions(context.Context) (*flaps.RegionData, error) {
	return &flaps.RegionData{Regions: []fly.Region{{Code: "iad"}, {Code: "ord"}}}, nil
}

func TestMultiRegionRecoveryUsesRequestedFlag(t *testing.T) {
	regionsOnce = sync.Once{}
	regionsFuture = nil
	t.Cleanup(func() { regionsOnce = sync.Once{}; regionsFuture = nil })
	ios, _, _, _ := iostreams.Test()
	ctx := iostreams.NewContext(context.Background(), ios)
	ctx = flapsutil.NewContextWithClient(ctx, recoveryRegionsClient{})
	flags := pflag.NewFlagSet("regions", pflag.ContinueOnError)
	flags.String("backup-regions", "", "")
	ctx = flag.NewContext(ctx, flags)
	regions, err := MultiRegion(ctx, "Choose regions", false, nil, nil, "backup-regions")
	require.Nil(t, regions)
	require.ErrorIs(t, err, ErrNonInteractive)
	require.ErrorContains(t, err, "--backup-regions")
	require.ErrorContains(t, err, "fly platform regions")
	require.NoError(t, flags.Set("backup-regions", "ord,iad"))
	regions, err = MultiRegion(ctx, "Choose regions", false, nil, nil, "backup-regions")
	require.NoError(t, err)
	require.Len(t, *regions, 2)
}
