package machine

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/flaps"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/flag"
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

func selectOneMachine(ctx context.Context, app *api.AppCompact, machineID string, haveMachineID bool) (*api.Machine, context.Context, error) {
	if err := checkSelectConditions(ctx, haveMachineID); err != nil {
		return nil, nil, err
	}

	var err error
	if app != nil {
		ctx, err = buildContextFromApp(ctx, app)
	} else {
		ctx, err = buildContextFromAppNameOrMachineID(ctx, machineID)
	}
	if err != nil {
		return nil, nil, err
	}

	var machine *api.Machine
	if shouldPrompt(ctx, haveMachineID) {
		machine, err = promptForOneMachine(ctx)
		if err != nil {
			return nil, nil, err
		}
	} else {
		machine, err = flaps.FromContext(ctx).Get(ctx, machineID)
		if err != nil {
			if err := rewriteMachineNotFoundErrors(ctx, err, machineID); err != nil {
				return nil, nil, err
			}
			return nil, nil, fmt.Errorf("could not get machine %s: %w", machineID, err)
		}
	}
	return machine, ctx, nil
}

func selectManyMachines(ctx context.Context, machineIDs []string) ([]*api.Machine, context.Context, error) {
	haveMachineIDs := len(machineIDs) > 0
	if err := checkSelectConditions(ctx, haveMachineIDs); err != nil {
		return nil, nil, err
	}

	ctx, err := buildContextFromAppNameOrMachineID(ctx, machineIDs...)
	if err != nil {
		return nil, nil, err
	}

	var machines []*api.Machine
	if shouldPrompt(ctx, haveMachineIDs) {
		machines, err = promptForManyMachines(ctx)
		if err != nil {
			return nil, nil, err
		}
	} else {
		flapsClient := flaps.FromContext(ctx)
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

func buildContextFromApp(ctx context.Context, app *api.AppCompact) (context.Context, error) {
	flapsClient, err := flaps.New(ctx, app)
	if err != nil {
		return nil, fmt.Errorf("could not create flaps client: %w", err)
	}
	ctx = flaps.NewContext(ctx, flapsClient)
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
		client := client.FromContext(ctx).API()
		var gqlMachine *api.GqlMachine
		gqlMachine, err = client.GetMachine(ctx, machineIDs[0])
		if err != nil {
			return nil, fmt.Errorf("could not get machine from GraphQL to determine app name: %w", err)
		}
		ctx = appconfig.WithName(ctx, gqlMachine.App.Name)
		flapsClient, err = flaps.New(ctx, gqlMachine.App)
	} else {
		flapsClient, err = flaps.NewFromAppName(ctx, appName)
	}
	if err != nil {
		return nil, fmt.Errorf("could not create flaps client: %w", err)
	}

	ctx = flaps.NewContext(ctx, flapsClient)
	return ctx, nil
}

func promptForOneMachine(ctx context.Context) (*api.Machine, error) {
	machines, err := flaps.FromContext(ctx).List(ctx, "")
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

func promptForManyMachines(ctx context.Context) ([]*api.Machine, error) {
	machines, err := flaps.FromContext(ctx).List(ctx, "")
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

	var selectedMachines []*api.Machine
	for _, selection := range selections {
		selectedMachines = append(selectedMachines, machines[selection])
	}
	if len(selectedMachines) == 0 {
		return nil, errors.New("no machines selected")
	}
	return selectedMachines, nil
}

func sortAndBuildOptions(machines []*api.Machine) []string {
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

func getMachineRole(machine *api.Machine) string {
	if machine.State != api.MachineStateStarted {
		return ""
	}
	for _, check := range machine.Checks {
		if check.Name == "role" {
			if check.Status == api.Passing {
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
