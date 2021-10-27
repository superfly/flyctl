package recipes

import (
	"context"
	"fmt"
	"strings"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/cmdctx"
	"github.com/superfly/flyctl/internal/recipe"
)

func PostgresAttachRecipe(cmdctx *cmdctx.CmdContext, app *api.App, input api.AttachPostgresClusterInput) error {
	ctx := cmdctx.Command.Context()

	recipe, err := recipe.NewRecipe(ctx, app)
	if err != nil {
		return err
	}

	machines, err := recipe.Client.API().ListMachines(ctx, input.PostgresClusterAppID, "started")
	if err != nil {
		return err
	}

	// Validate database name
	dbVerifySQL := EncodeCommand(fmt.Sprintf("select exists(select datname from pg_catalog.pg_database WHERE datname = '%s');", *input.DatabaseName))
	dbVerifyCmd := []string{PG_RUN_SQL_SCRIPT, "-database", "postgres", "-command", dbVerifySQL}
	dbVerifyOp, err := recipe.RunSSHOperation(ctx, machines[0], strings.Join(dbVerifyCmd, " "))
	if err != nil {
		return err
	}
	if dbVerifyOp.Message == "t" {
		return fmt.Errorf("Database %q already exists...", *input.DatabaseName)
	}

	// Validate user name
	userVerifySQL := EncodeCommand(fmt.Sprintf("select exists (select from pg_roles where rolname = '%s');", *input.DatabaseUser))
	userVerifyCmd := []string{PG_RUN_SQL_SCRIPT, "-database", "postgres", "-command", userVerifySQL}
	userVerifyOp, err := recipe.RunSSHOperation(ctx, machines[0], strings.Join(userVerifyCmd, " "))
	if err != nil {
		return err
	}
	if userVerifyOp.Message == "t" {
		return fmt.Errorf("User %q already exists...", *input.DatabaseUser)
	}

	// Create database
	dbCreateSQL := EncodeCommand(fmt.Sprintf("CREATE DATABASE %s", *input.DatabaseName))
	dbCreateCmd := []string{PG_RUN_SQL_SCRIPT, "-database", "postgres", "-command", dbCreateSQL}
	_, err = recipe.RunSSHOperation(ctx, machines[0], strings.Join(dbCreateCmd, " "))
	if err != nil {
		return err
	}

	// Generate user
	password := GenerateSecureToken(15)
	createUserSQL := EncodeCommand(fmt.Sprintf("CREATE USER %s WITH ENCRYPTED PASSWORD '%s' LOGIN;", *input.DatabaseUser, password))
	createUserCmd := []string{PG_RUN_SQL_SCRIPT, "-database", *input.DatabaseName, "-command", createUserSQL}

	_, err = recipe.RunSSHOperation(ctx, machines[0], strings.Join(createUserCmd, " "))
	if err != nil {
		cleanUp(ctx, recipe, machines[0], input)
		return err
	}

	_, err = cmdctx.Client.API().AttachPostgresCluster(ctx, input)
	if err != nil {
		cleanUp(ctx, recipe, machines[0], input)
		return err
	}

	connectionString := fmt.Sprintf("postgres://%s:%s@%s.internal:5432/%s", *input.DatabaseUser, password, input.PostgresClusterAppID, *input.DatabaseName)

	secrets := map[string]string{}
	secrets[*input.VariableName] = connectionString

	_, err = cmdctx.Client.API().SetSecrets(ctx, input.AppID, secrets)
	if err != nil {
		return err
	}

	fmt.Printf("Postgres cluster %s is now attached to %s\n", input.PostgresClusterAppID, input.AppID)
	fmt.Printf("The following secret as added to %s\n", input.AppID)
	fmt.Printf("%s=%s\n", *input.VariableName, connectionString)

	return nil
}

func cleanUp(ctx context.Context, recipe *recipe.Recipe, machine *api.Machine, input api.AttachPostgresClusterInput) {
	dbDropSQL := EncodeCommand(fmt.Sprintf("DROP DATABASE %s IF EXISTS", *input.DatabaseName))
	dbDropCmd := fmt.Sprintf("%s -database postgres -command %s", PG_RUN_SQL_SCRIPT, dbDropSQL)
	_, err := recipe.RunSSHOperation(ctx, machine, dbDropCmd)
	if err != nil {
		fmt.Printf("Failed to drop database %q. %v", *input.DatabaseName, err)
	}

	userDropSQL := EncodeCommand(fmt.Sprintf("DROP USER %s IF EXISTS", *input.DatabaseUser))
	userDropCmd := fmt.Sprintf("%s -database postgres -command %s", PG_RUN_SQL_SCRIPT, userDropSQL)
	_, err = recipe.RunSSHOperation(ctx, machine, userDropCmd)
	if err != nil {
		fmt.Printf("Failed to drop user %q. %v", *input.DatabaseUser, err)
	}

}
