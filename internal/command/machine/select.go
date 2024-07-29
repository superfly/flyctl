package machine

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	fly "github.com/superfly/fly-go"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/prompt"
	"github.com/superfly/flyctl/iostreams"
)

// We now prompt for a machine automatically when no machine IDs are
// provided. This flag is retained for backward compatability.
var selectFlag = flag.Bool{
	Name:        "select",
	Description: "Select from a list of machines",
	Hidden:      true,
}

func selectOneMachine(ctx context.Context, appName string, machineID string, haveMachineID bool) (*fly.Machine, context.Context, error) {
	if err := checkSelectConditions(ctx, haveMachineID); err != nil {
		return nil, nil, err
	}

	var err error
	if appName != "" {
		ctx, err = buildContextFromAppName(ctx, appName)
	} else {
		ctx, err = buildContextFromAppNameOrMachineID(ctx, machineID)
	}
	if err != nil {
		return nil, nil, err
	}

	var machine *fly.Machine
	if shouldPrompt(ctx, haveMachineID) {
		machine, err = promptForOneMachine(ctx)
		if err != nil {
			return nil, nil, err
		}
	} else {
		machine, err = flapsutil.ClientFromContext(ctx).Get(ctx, machineID)
		if err != nil {
			if err := rewriteMachineNotFoundErrors(ctx, err, machineID); err != nil {
				return nil, nil, err
			}
			return nil, nil, fmt.Errorf("could not get machine %s: %w", machineID, err)
		}
	}
	return machine, ctx, nil
}

func selectManyMachines(ctx context.Context, machineIDs []string) ([]*fly.Machine, context.Context, error) {
	haveMachineIDs := len(machineIDs) > 0
	if err := checkSelectConditions(ctx, haveMachineIDs); err != nil {
		return nil, nil, err
	}

	ctx, err := buildContextFromAppNameOrMachineID(ctx, machineIDs...)
	if err != nil {
		return nil, nil, err
	}

	var machines []*fly.Machine
	if shouldPrompt(ctx, haveMachineIDs) {
		machines, err = promptForManyMachines(ctx)
		if err != nil {
			return nil, nil, err
		}
	} else {
		flapsClient := flapsutil.ClientFromContext(ctx)
		for _, machineID := range machineIDs {
			machine, err := flapsClient.Get(ctx, machineID)
			if err != nil {
				if err := rewriteMachineNotFoundErrors(ctx, err, machineID); err != nil {
					return nil, nil, err
				}
				return nil, nil, fmt.Errorf("could not get machine %s: %w", machineID, err)
			}
			machines = append(machines, machine)
		}
	}
	return machines, ctx, nil
}

func selectManyMachineIDs(ctx context.Context, machineIDs []string) ([]string, context.Context, error) {
	haveMachineIDs := len(machineIDs) > 0
	if err := checkSelectConditions(ctx, haveMachineIDs); err != nil {
		return nil, nil, err
	}

	ctx, err := buildContextFromAppNameOrMachineID(ctx, machineIDs...)
	if err != nil {
		return nil, nil, err
	}

	if shouldPrompt(ctx, haveMachineIDs) {
		// NOTE: machineIDs must be empty in this case.
		machines, err := promptForManyMachines(ctx)
		if err != nil {
			return nil, nil, err
		}
		for _, machine := range machines {
			machineIDs = append(machineIDs, machine.ID)
		}
	}
	return machineIDs, ctx, nil
}

func buildContextFromAppName(ctx context.Context, appName string) (context.Context, error) {
	flapsClient, err := flapsutil.NewClientWithOptions(ctx, flaps.NewClientOpts{
		AppName: appName,
	})
	if err != nil {
		return nil, fmt.Errorf("could not create flaps client: %w", err)
	}
	ctx = flapsutil.NewContextWithClient(ctx, flapsClient)
	return ctx, nil
}

func buildContextFromAppNameOrMachineID(ctx context.Context, machineIDs ...string) (context.Context, error) {
	var (
		appName = appconfig.NameFromContext(ctx)

		flapsClient *flaps.Client
		err         error
	)

	if appName == "" {
		// NOTE: assuming that we validated the command line arguments
		// correctly, we must have at least one machine ID when no app
		// is set.
		client := flyutil.ClientFromContext(ctx)
		var gqlMachine *fly.GqlMachine
		gqlMachine, err = client.GetMachine(ctx, machineIDs[0])
		if err != nil {
			return nil, fmt.Errorf("could not get machine from GraphQL to determine app name: %w", err)
		}
		ctx = appconfig.WithName(ctx, gqlMachine.App.Name)
		flapsClient, err = flapsutil.NewClientWithOptions(ctx, flaps.NewClientOpts{
			AppCompact: gqlMachine.App,
			AppName:    gqlMachine.App.Name,
		})
	} else {
		flapsClient, err = flapsutil.NewClientWithOptions(ctx, flaps.NewClientOpts{
			AppName: appName,
		})
	}
	if err != nil {
		return nil, fmt.Errorf("could not create flaps client: %w", err)
	}

	ctx = flapsutil.NewContextWithClient(ctx, flapsClient)
	return ctx, nil
}

func promptForOneMachine(ctx context.Context) (*fly.Machine, error) {
	machines, err := flapsutil.ClientFromContext(ctx).List(ctx, "")
	if err != nil {
		return nil, fmt.Errorf("could not get a list of machines: %w", err)
	} else if len(machines) == 0 {
		return nil, fmt.Errorf("the app %s has no machines", appconfig.NameFromContext(ctx))
	}

	options := sortAndBuildOptions(machines)
	var selection int
	if err := prompt.Select(ctx, &selection, "Select a machine:", "", options...); err != nil {
		return nil, fmt.Errorf("could not prompt for machine: %w", err)
	}
	return machines[selection], nil
}

func promptForManyMachines(ctx context.Context) ([]*fly.Machine, error) {
	machines, err := flapsutil.ClientFromContext(ctx).List(ctx, "")
	if err != nil {
		return nil, fmt.Errorf("could not get a list of machines: %w", err)
	} else if len(machines) == 0 {
		return nil, fmt.Errorf("the app %s has no machines", appconfig.NameFromContext(ctx))
	}

	options := sortAndBuildOptions(machines)
	var selections []int
	if err := prompt.MultiSelect(ctx, &selections, "Select machines:", nil, options...); err != nil {
		return nil, fmt.Errorf("could not prompt for machines: %w", err)
	}

	var selectedMachines []*fly.Machine
	for _, selection := range selections {
		selectedMachines = append(selectedMachines, machines[selection])
	}
	if len(selectedMachines) == 0 {
		return nil, errors.New("no machines selected")
	}
	return selectedMachines, nil
}

func sortAndBuildOptions(machines []*fly.Machine) []string {
	sort.Slice(machines, func(i, j int) bool {
		return machines[i].ID < machines[j].ID
	})

	options := []string{}
	for _, machine := range machines {
		details := fmt.Sprintf("%s, region %s", machine.State, machine.Region)
		if group := machine.ProcessGroup(); group != "" {
			details += fmt.Sprintf(", process group '%s'", group)
		}
		role := getMachineRole(machine)
		if role != "" {
			details += fmt.Sprintf(", role '%s'", role)
		}
		options = append(options, fmt.Sprintf("%s %s (%s)", machine.ID, machine.Name, details))
	}
	return options
}

func getMachineRole(machine *fly.Machine) string {
	if machine.State != fly.MachineStateStarted {
		return ""
	}
	for _, check := range machine.Checks {
		if check.Name == "role" {
			if check.Status == fly.Passing {
				return check.Output
			} else {
				return "error"
			}
		}
	}
	return ""
}

func rewriteMachineNotFoundErrors(ctx context.Context, err error, machineID string) error {
	if strings.Contains(err.Error(), "machine not found") {
		appName := appconfig.NameFromContext(ctx)
		return fmt.Errorf("machine %s was not found in app '%s'", machineID, appName)
	} else {
		return nil
	}
}

func checkSelectConditions(ctx context.Context, haveMachineIDs bool) error {
	haveSelectFlag := flag.GetBool(ctx, "select")
	appName := appconfig.NameFromContext(ctx)
	switch {
	case haveSelectFlag && haveMachineIDs:
		return errors.New("machine IDs can't be used with --select")
	case haveSelectFlag && appName == "":
		return errors.New("an app name must be specified to use --select")
	case !haveMachineIDs && appName == "":
		return errors.New("a machine ID or an app name is required")
	case shouldPrompt(ctx, haveMachineIDs) && !iostreams.FromContext(ctx).IsInteractive():
		return errors.New("a machine ID must be specified when not running interactively")
	default:
		return nil
	}
}

func shouldPrompt(ctx context.Context, haveMachineIDs bool) bool {
	return flag.GetBool(ctx, "select") || !haveMachineIDs
}
