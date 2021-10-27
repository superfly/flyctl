package recipes

import (
	"fmt"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/cmdctx"
	"github.com/superfly/flyctl/internal/recipe"
)

type PostgresConnectInput struct {
	App      *api.App
	Username string
	Password string
	Database string
}

func PostgresConnectRecipe(cmdctx *cmdctx.CmdContext, input *PostgresConnectInput) error {
	ctx := cmdctx.Command.Context()

	recipe, err := recipe.NewRecipe(ctx, input.App)
	if err != nil {
		return err
	}

	machines, err := recipe.Client.API().ListMachines(ctx, input.App.ID, "started")
	if err != nil {
		return err
	}

	cmd := fmt.Sprintf("%s %s %s %s", PG_CONNECT_SCRIPT, input.Database, input.Username, input.Password)

	_, err = recipe.RunSSHAttachOperation(ctx, machines[0], cmd)
	if err != nil {
		return err
	}

	return nil
}
