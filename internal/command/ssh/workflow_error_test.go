package ssh

import (
	"context"
	"errors"
	"fmt"
	"testing"

	surveyterminal "github.com/AlecAivazis/survey/v2/terminal"
	"github.com/spf13/pflag"
	"github.com/stretchr/testify/require"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/mock"
	"github.com/superfly/flyctl/internal/prompt"
	"github.com/superfly/flyctl/internal/tracing"
	"github.com/superfly/flyctl/iostreams"
)

type workflowSelectionClient struct {
	flapsutil.FlapsClient
	machines []*fly.Machine
	err      error
}

func (client workflowSelectionClient) ListActive(context.Context, string) ([]*fly.Machine, error) {
	return client.machines, client.err
}

// Ensure actual Machine-selection branches assign distinct fixed codes after
// successful listings, including filtered lists, target mismatches, and prompts.
func TestMachineSelectionWorkflowCodes(t *testing.T) {
	started := &fly.Machine{ID: "private-machine", Name: "private-name", State: "started", Region: "iad", Config: &fly.MachineConfig{Metadata: map[string]string{"fly_process_group": "app"}}}
	other := &fly.Machine{ID: "other-machine", State: "started", Region: "ord", Config: &fly.MachineConfig{Metadata: map[string]string{"fly_process_group": "worker"}}}
	stopped := &fly.Machine{ID: "stopped-machine", State: "stopped"}
	listErr := errors.New("private list error")
	for _, tt := range []struct {
		name       string
		machines   []*fly.Machine
		args       []string
		listErr    error
		lookup     *fly.GqlMachine
		lookupErr  error
		code       tracing.WorkflowErrorCode
		diagnostic string
	}{
		{name: "list failed", listErr: listErr, code: tracing.WorkflowErrorSSHMachineList, diagnostic: listErr.Error()},
		{name: "empty listing", code: tracing.WorkflowErrorSSHNoStartedMachines, diagnostic: "has no started VMs"},
		{name: "only stopped Machines", machines: []*fly.Machine{stopped}, code: tracing.WorkflowErrorSSHNoStartedMachines, diagnostic: "has no started VMs"},
		{name: "requested stopped Machine", machines: []*fly.Machine{stopped}, args: []string{"--machine", stopped.ID}, code: tracing.WorkflowErrorSSHMachineNotStarted, diagnostic: "is not started"},
		{name: "requested missing Machine", args: []string{"--machine", "missing"}, code: tracing.WorkflowErrorSSHMachineNotFound, diagnostic: "was not found"},
		{name: "target lookup failed", args: []string{"--machine", "missing"}, lookupErr: errors.New("private lookup error"), code: tracing.WorkflowErrorSSHMachineLookup, diagnostic: "was not found"},
		{name: "target in another app", args: []string{"--machine", "missing"}, lookup: &fly.GqlMachine{App: &fly.AppCompact{Name: "other-app"}}, code: tracing.WorkflowErrorSSHMachineAppMismatch, diagnostic: "belongs to app"},
		{name: "region has no started Machines", machines: []*fly.Machine{started}, args: []string{"--region", "ord"}, code: tracing.WorkflowErrorSSHNoMachinesInRegion, diagnostic: "has no VMs in region"},
		{name: "target region mismatch", machines: []*fly.Machine{started, other}, args: []string{"--machine", started.ID, "--region", other.Region}, code: tracing.WorkflowErrorSSHMachineRegionMismatch, diagnostic: "not selected region"},
		{name: "process group empty", machines: []*fly.Machine{started}, args: []string{"--process-group", "worker"}, code: tracing.WorkflowErrorSSHNoMachinesInProcessGroup, diagnostic: "has no VMs in process group"},
		{name: "target process group mismatch", machines: []*fly.Machine{started, other}, args: []string{"--machine", started.ID, "--process-group", "worker"}, code: tracing.WorkflowErrorSSHMachineProcessGroupMismatch, diagnostic: "not selected process group"},
		{name: "target and selection flags", machines: []*fly.Machine{started}, args: []string{"--machine", started.ID, "--select"}, code: tracing.WorkflowErrorSSHSelectionFlags, diagnostic: "can't be used"},
		{name: "selection needs interactive input", machines: []*fly.Machine{started, other}, args: []string{"--select"}, code: tracing.WorkflowErrorPromptNonInteractive, diagnostic: "selecting VM"},
		{name: "successful selection", machines: []*fly.Machine{started}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			flags := pflag.NewFlagSet("ssh", pflag.ContinueOnError)
			flags.String("machine", "", "")
			flags.String("region", "", "")
			flags.String("process-group", "", "")
			flags.Bool("select", false, "")
			require.NoError(t, flags.Parse(tt.args))
			streams, _, _, _ := iostreams.Test()
			ctx := flag.NewContext(iostreams.NewContext(context.Background(), streams), flags)
			ctx = flapsutil.NewContextWithClient(ctx, workflowSelectionClient{machines: tt.machines, err: tt.listErr})
			ctx = flyutil.NewContextWithClient(ctx, &mock.Client{GetMachineFunc: func(context.Context, string) (*fly.GqlMachine, error) { return tt.lookup, tt.lookupErr }})
			machine, err := selectMachine(ctx, &flaps.App{Name: "private-app"})
			if tt.diagnostic == "" {
				require.NoError(t, err)
				require.Same(t, started, machine)
				return
			}
			require.ErrorContains(t, err, tt.diagnostic)
			require.Nil(t, machine)
			require.Equal(t, tt.code, tracing.WorkflowErrorCodeFor(err))
			if tt.listErr != nil {
				require.ErrorIs(t, err, tt.listErr)
			}
		})
	}
}

// Ensure typed prompt outcomes survive wrapping; an arbitrary message mentioning
// cancellation cannot be mistaken for an explicit interruption.
func TestWorkflowPromptCodes(t *testing.T) {
	for _, tt := range []struct {
		err  error
		code tracing.WorkflowErrorCode
	}{
		{fmt.Errorf("wrapped: %w", surveyterminal.InterruptErr), tracing.WorkflowErrorInterrupted},
		{fmt.Errorf("wrapped: %w", prompt.ErrNonInteractive), tracing.WorkflowErrorPromptNonInteractive},
		{errors.New("interrupt"), tracing.WorkflowErrorSSHMachinePrompt},
	} {
		err := workflowPromptError(tt.err, tracing.WorkflowErrorSSHMachinePrompt)
		require.Equal(t, tt.err.Error(), err.Error())
		require.ErrorIs(t, err, tt.err)
		require.Equal(t, tt.code, tracing.WorkflowErrorCodeFor(err))
	}
}
