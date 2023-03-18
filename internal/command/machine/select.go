package machine

import (
	"context"
	"fmt"
	"strings"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/flaps"
	"github.com/superfly/flyctl/internal/appconfig"
)

func selectOneMachine(ctx context.Context, app *api.AppCompact, machineID string) (*api.Machine, context.Context, error) {
	var err error
	if app != nil {
		ctx, err = buildContextFromApp(ctx, app)
	} else {
		ctx, err = buildContextFromAppNameOrMachineID(ctx, machineID)
	}
	if err != nil {
		return nil, nil, err
	}

	machine, err := flaps.FromContext(ctx).Get(ctx, machineID)
	if err != nil {
		if err := rewriteMachineNotFoundErrors(ctx, err, machineID); err != nil {
			return nil, nil, err
		}
		return nil, nil, fmt.Errorf("could not get machine %s: %w", machineID, err)
	}
	return machine, ctx, nil
}

func selectManyMachines(ctx context.Context, machineIDs []string) ([]*api.Machine, context.Context, error) {
	ctx, err := buildContextFromAppNameOrMachineID(ctx, machineIDs[0])
	if err != nil {
		return nil, nil, err
	}
	flapsClient := flaps.FromContext(ctx)

	var machines []*api.Machine
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
	return machines, ctx, nil
}

func selectManyMachineIDs(ctx context.Context, machineIDs []string) ([]string, context.Context, error) {
	ctx, err := buildContextFromAppNameOrMachineID(ctx, machineIDs[0])
	if err != nil {
		return nil, nil, err
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

func buildContextFromAppNameOrMachineID(ctx context.Context, machineID string) (context.Context, error) {
	var (
		appName = appconfig.NameFromContext(ctx)

		flapsClient *flaps.Client
		err         error
	)

	if appName == "" {
		client := client.FromContext(ctx).API()
		var gqlMachine *api.GqlMachine
		gqlMachine, err = client.GetMachine(ctx, machineID)
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

func rewriteMachineNotFoundErrors(ctx context.Context, err error, machineID string) error {
	if strings.Contains(err.Error(), "machine not found") {
		appName := appconfig.NameFromContext(ctx)
		return fmt.Errorf("machine %s was not found in app '%s'", machineID, appName)
	} else {
		return nil
	}
}
