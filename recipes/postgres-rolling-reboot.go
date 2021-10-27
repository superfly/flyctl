package recipes

import (
	"context"
	"fmt"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/internal/recipe"
)

func PostgresRollingRebootRecipe(ctx context.Context, app *api.App) error {

	recipe, err := recipe.NewRecipe(ctx, app)
	if err != nil {
		return err
	}

	machines, err := recipe.Client.API().ListMachines(ctx, app.ID, "")
	if err != nil {
		fmt.Println(err.Error())
	}

	roleMap := map[string][]*api.Machine{}

	// Collect PG role information from each machine
	for _, machine := range machines {
		stateOp, err := recipe.RunSSHOperation(ctx, machine, PG_ROLE_SCRIPT)
		if err != nil {
			return err
		}
		roleMap[stateOp.Message] = append(roleMap[stateOp.Message], stateOp.Machine)
	}

	// Restart replicas
	for _, machine := range roleMap["replica"] {
		_, err = recipe.RunSSHOperation(ctx, machine, PG_RESTART_SCRIPT)
		if err != nil {
			return err
		}
	}

	// Failover and restart leader
	for _, machine := range roleMap["leader"] {
		_, err = recipe.RunSSHOperation(ctx, machine, PG_FAILOVER_SCRIPT)
		if err != nil {
			return err
		}

		_, err = recipe.RunSSHOperation(ctx, machine, PG_RESTART_SCRIPT)
		if err != nil {
			return err
		}
	}

	return nil
}
