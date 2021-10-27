package recipes

import (
	"context"
	"fmt"
	"net/http"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/internal/recipe"
)

type PostgresUpgradeConfig struct {
	AppName        string
	TargetImageRef string
}

func PostgresImageUpgradeRecipe(ctx context.Context, app *api.App, image string) error {
	recipe, err := recipe.NewRecipe(ctx, app)
	if err != nil {
		return err
	}

	// TODO - ListMachines should exclude removed machines. Once this is fixed, we should not
	// filter machines based on state.
	machines, err := recipe.Client.API().ListMachines(ctx, app.Name, "started")
	if err != nil {
		return err
	}

	// Collect PG role information from each machine
	roleMap := map[string][]*api.Machine{}
	for _, machine := range machines {
		stateOp, err := recipe.RunSSHOperation(ctx, machine, PG_ROLE_SCRIPT)
		if err != nil {
			return err
		}
		roleMap[stateOp.Message] = append(roleMap[stateOp.Message], stateOp.Machine)
	}

	// Use a non-cancelable context from here on out, as it's somewhat dangerous to cancel
	// the upgrade process after this point.
	ctx = context.TODO()

	// Destroy replica and replace it with new machine w/ desired image.
	for _, machine := range roleMap["replica"] {

		if err = destroyMachine(ctx, recipe, machine); err != nil {
			return err
		}

		newMachine, err := replaceMachine(ctx, recipe, app, machine, image)
		if err != nil {
			return err
		}

		_, err = recipe.RunSSHOperation(ctx, newMachine, PG_IS_HEALTHY_SCRIPT)
		if err != nil {
			return err
		}
	}

	// Initiate failover, destroy old leader and replace it.
	for _, machine := range roleMap["leader"] {
		_, err = recipe.RunSSHOperation(ctx, machine, PG_FAILOVER_SCRIPT)
		if err != nil {
			return err
		}

		if err = destroyMachine(ctx, recipe, machine); err != nil {
			return err
		}

		newMachine, err := replaceMachine(ctx, recipe, app, machine, image)
		if err != nil {
			return err
		}

		_, err = recipe.RunSSHOperation(ctx, newMachine, PG_IS_HEALTHY_SCRIPT)
		if err != nil {
			return err
		}
	}

	return nil
}

func destroyMachine(ctx context.Context, recipe *recipe.Recipe, machine *api.Machine) error {
	stopEndpoint := fmt.Sprintf("/v1/machines/%s/stop", machine.ID)
	_, err := recipe.RunHTTPOperation(ctx, machine, http.MethodPost, stopEndpoint)
	if err != nil {
		return err
	}

	destroyEndpoint := fmt.Sprintf("/v1/machines/%s/", machine.ID)
	_, err = recipe.RunHTTPOperation(ctx, machine, http.MethodDelete, destroyEndpoint)
	if err != nil {
		return err
	}

	return nil
}

func replaceMachine(ctx context.Context, recipe *recipe.Recipe, app *api.App, machine *api.Machine, image string) (*api.Machine, error) {
	newConfig := machine.Config
	newConfig["image"] = image

	launchInput := api.LaunchMachineInput{
		AppID:  app.ID,
		Region: machine.Region,
		Config: &newConfig,
	}

	fmt.Printf("Launching new machine to replace %s\n", machine.ID)

	m, _, err := recipe.Client.API().LaunchMachine(ctx, launchInput)
	if err != nil {
		return nil, err
	}

	WaitForMachineState(ctx, recipe.Client, app.ID, m.ID, "started")

	return m, nil
}
