package recipes

import (
	"fmt"
	"strings"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/cmdctx"
	"github.com/superfly/flyctl/internal/recipe"
)

func PostgresDetachRecipe(cmdctx *cmdctx.CmdContext, app *api.App, pgApp *api.App) error {
	ctx := cmdctx.Command.Context()

	recipe, err := recipe.NewRecipe(ctx, app)
	if err != nil {
		return err
	}

	fmt.Println("Detaching")
	// Remove attachment from Web.
	detach, err := recipe.Client.API().DetachPostgresCluster(ctx, pgApp.Name, app.Name)
	if err != nil {
		return err
	}

	fmt.Printf("Detach info: %+v", detach)

	return fmt.Errorf("Premature failure")

	machines, err := recipe.Client.API().ListMachines(ctx, pgApp.ID, "started")
	if err != nil {
		return err
	}

	// Remove database
	dbDropSQL := EncodeCommand(fmt.Sprintf("DROP DATABASE %s IF EXISTS", detach.DatabaseName))
	dbDropCmd := []string{PG_RUN_SQL_SCRIPT, "-database", "postgres", "-command", dbDropSQL}
	_, err = recipe.RunSSHOperation(ctx, machines[0], strings.Join(dbDropCmd, " "))
	if err != nil {
		fmt.Printf("Failed to drop database %q. %v", detach.DatabaseName, err)
	}

	// Remove user
	userDropSQL := EncodeCommand(fmt.Sprintf("DROP USER %s IF EXISTS", detach.DatabaseUser))
	userDropCmd := []string{PG_RUN_SQL_SCRIPT, "-database", "postgres", "-command", userDropSQL}
	_, err = recipe.RunSSHOperation(ctx, machines[0], strings.Join(userDropCmd, " "))
	if err != nil {
		fmt.Printf("Failed to drop user %q. %v", detach.DatabaseUser, err)
	}

	secrets := []string{detach.EnvironmentVariableName}
	_, err = cmdctx.Client.API().UnsetSecrets(ctx, detach.App.Name, secrets)
	if err != nil {
		return err
	}

	fmt.Printf("Postgres cluster %s has been detached from %s\n", detach.PostgresClusterApp.Name, detach.App.Name)

	return nil
}
