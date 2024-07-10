package scan

import (
	"context"
	"errors"
	"fmt"

	"github.com/samber/lo"
	"github.com/spf13/cobra"

	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/prompt"
)

func New() *cobra.Command {
	const (
		usage = "scan"
		short = "Scan machine images for vulnerabilities or to get an SBOM"
		long  = "Scan machine images for vulnerabilities or to get an SBOM."
	)
	cmd := command.New(usage, short, long, nil)

	cmd.AddCommand(
		newSbom(),
		newVulns(),
	)

	return cmd
}

func selectMachine(ctx context.Context, app *fly.AppCompact) (*fly.Machine, error) {
	if flag.IsSpecified(ctx, "machine") {
		if flag.IsSpecified(ctx, "select") {
			return nil, errors.New("--machine can't be used with -s/--select")
		}
		return getMachineByID(ctx)
	}
	return promptForMachine(ctx, app)
}

func getMachineByID(ctx context.Context) (*fly.Machine, error) {
	flapsClient := flapsutil.ClientFromContext(ctx)
	machineID := flag.GetString(ctx, "machine")
	machine, err := flapsClient.Get(ctx, machineID)
	if err != nil {
		return nil, err
	}
	if machine.State != fly.MachineStateStarted {
		return nil, fmt.Errorf("machine %s is not started", machineID)
	}

	return machine, nil
}

func promptForMachine(ctx context.Context, app *fly.AppCompact) (*fly.Machine, error) {
	anyMachine := !flag.IsSpecified(ctx, "select")

	flapsClient := flapsutil.ClientFromContext(ctx)
	machines, err := flapsClient.ListActive(ctx)
	if err != nil {
		return nil, err
	}

	// TODO: Perhaps we should allow selection of non-started machines,
	// preferring the started machines over the non-started machines.
	machines = lo.Filter(machines, func(machine *fly.Machine, _ int) bool {
		return machine.State == fly.MachineStateStarted
	})

	options := []string{}
	for _, machine := range machines {
		// TODO: show state? show imageref?
		options = append(options, fmt.Sprintf("%s: %s %s", machine.Region, machine.ID, machine.Name))
	}

	if len(machines) == 0 {
		return nil, fmt.Errorf("there are no running machines")
	}

	if anyMachine || len(machines) == 1 {
		return machines[0], nil
	}

	index := 0
	if err := prompt.Select(ctx, &index, "Select a machine:", "", options...); err != nil {
		return nil, fmt.Errorf("failed to prompt for a machine: %w", err)
	}
	return machines[index], nil
}
