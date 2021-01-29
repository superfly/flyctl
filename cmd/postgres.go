package cmd

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/briandowns/spinner"
	"github.com/logrusorgru/aurora"
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/cmd/presenters"
	"github.com/superfly/flyctl/cmdctx"
	"github.com/superfly/flyctl/docstrings"
	"github.com/superfly/flyctl/helpers"
)

func newPostgresCommand() *Command {
	domainsStrings := docstrings.Get("postgres")
	cmd := BuildCommandKS(nil, nil, domainsStrings, os.Stdout, requireSession)
	cmd.Hidden = true

	listStrings := docstrings.Get("postgres.list")
	listCmd := BuildCommandKS(cmd, runPostgresList, listStrings, os.Stdout, requireSession)
	listCmd.Args = cobra.MaximumNArgs(1)

	createStrings := docstrings.Get("postgres.create")
	createCmd := BuildCommandKS(cmd, runCreatePostgresCluster, createStrings, os.Stdout, requireSession)
	createCmd.AddStringFlag(StringFlagOpts{Name: "organization", Description: "the organization that will own the app"})
	createCmd.AddStringFlag(StringFlagOpts{Name: "name", Description: "the name of the new app"})
	createCmd.AddStringFlag(StringFlagOpts{Name: "region", Description: "the region to launch the new app in"})
	createCmd.AddStringFlag(StringFlagOpts{Name: "password", Description: "the superuser password. one will be generated for you if you leave this blank"})

	attachStrngs := docstrings.Get("postgres.attach")
	attachCmd := BuildCommandKS(cmd, runAttachPostgresCluster, attachStrngs, os.Stdout, requireSession, requireAppName)
	attachCmd.AddStringFlag(StringFlagOpts{Name: "postgres-app", Description: "the postgres cluster to attach to the app"})
	attachCmd.AddStringFlag(StringFlagOpts{Name: "database-name", Description: "database to use, defaults to a new database with the same name as the app"})
	attachCmd.AddStringFlag(StringFlagOpts{Name: "variable-name", Description: "the env variable name that will be added to the app. Defaults to DATABASE_URL"})

	detachStrngs := docstrings.Get("postgres.detach")
	detachCmd := BuildCommandKS(cmd, runDetachPostgresCluster, detachStrngs, os.Stdout, requireSession, requireAppName)
	detachCmd.AddStringFlag(StringFlagOpts{Name: "postgres-app", Description: "the postgres cluster to detach from the app"})

	dbStrings := docstrings.Get("postgres.db")
	dbCmd := BuildCommandKS(cmd, nil, dbStrings, os.Stdout, requireSession)

	listDBStrings := docstrings.Get("postgres.db.list")
	listDBCmd := BuildCommandKS(dbCmd, runListPostgresDatabases, listDBStrings, os.Stdout, requireSession, requireAppNameAsArg)
	listDBCmd.Args = cobra.ExactArgs(1)

	usersStrings := docstrings.Get("postgres.users")
	usersCmd := BuildCommandKS(cmd, nil, usersStrings, os.Stdout, requireSession)

	usersListStrings := docstrings.Get("postgres.users.list")
	usersListCmd := BuildCommandKS(usersCmd, runListPostgresUsers, usersListStrings, os.Stdout, requireSession, requireAppNameAsArg)
	usersListCmd.Args = cobra.ExactArgs(1)

	return cmd
}

func runPostgresList(ctx *cmdctx.CmdContext) error {
	apps, err := ctx.Client.API().GetApps(api.StringPointer("postgres_cluster"))
	if err != nil {
		return err
	}

	if ctx.OutputJSON() {
		ctx.WriteJSON(apps)
		return nil
	}

	return ctx.Render(&presenters.Apps{Apps: apps})
}

func runCreatePostgresCluster(ctx *cmdctx.CmdContext) error {
	name, _ := ctx.Config.GetString("name")
	if name == "" {
		return errors.New("name is required")
	}

	region, _ := ctx.Config.GetString("region")

	orgSlug, _ := ctx.Config.GetString("organization")
	org, err := selectOrganization(ctx.Client.API(), orgSlug)
	if err != nil {
		return err
	}

	input := api.CreatePostgresClusterInput{
		OrganizationID: org.ID,
		Name:           name,
		Region:         api.StringPointer(region),
	}

	fmt.Fprintf(ctx.Out, "Creating postgres cluster %s in organization %s\n", name, org.Slug)

	s := spinner.New(spinner.CharSets[11], 100*time.Millisecond)
	s.Writer = os.Stderr
	s.Prefix = "Launching..."
	s.Start()

	payload, err := ctx.Client.API().CreatePostgresCluster(input)
	if err != nil {
		return err
	}

	s.FinalMSG = fmt.Sprintf("Postgres cluster %s created\n", payload.App.Name)
	s.Stop()

	fmt.Printf("  Username:    %s\n", payload.Username)
	fmt.Printf("  Password:    %s\n", payload.Password)
	fmt.Printf("  Hostname:    %s.internal\n", payload.App.Name)
	fmt.Printf("  Proxy Port:  5432\n")
	fmt.Printf("  PG Port: 5433\n")

	fmt.Println(aurora.Italic("Save your credentials in a secure place, you won't be able to see them again!"))
	fmt.Println()

	fmt.Println(aurora.Bold("Connect to postgres"))
	fmt.Printf("Any app within the %s organization can connect to postgres using the above credentials and the hostname \"%s.internal.\"\n", org.Slug, payload.App.Name)
	fmt.Printf("For example: postgres://%s:%s@%s.internal:%d\n", payload.Username, payload.Password, payload.App.Name, 5432)

	fmt.Println()
	fmt.Println("See the postgres docs for more information on next steps, managing postgres, connecting from outside fly:  https://fly.io/docs/reference/postgres/")

	return nil
}

func runAttachPostgresCluster(ctx *cmdctx.CmdContext) error {
	postgresAppName, _ := ctx.Config.GetString("postgres-app")
	appName := ctx.AppName

	input := api.AttachPostgresClusterInput{
		AppID:                appName,
		PostgresClusterAppID: postgresAppName,
	}

	if dbName, _ := ctx.Config.GetString("database-name"); dbName != "" {
		input.DatabaseName = api.StringPointer(dbName)
	}
	if varName, _ := ctx.Config.GetString("variable-name"); varName != "" {
		input.VariableName = api.StringPointer(varName)
	}

	s := spinner.New(spinner.CharSets[11], 100*time.Millisecond)
	s.Writer = os.Stderr
	s.Prefix = "Attaching..."
	s.Start()

	app, postgresApp, err := ctx.Client.API().AttachPostgresCluster(input)

	if err != nil {
		return err
	}

	s.FinalMSG = fmt.Sprintf("Postgres cluster %s is now attached to %s\n", postgresApp.Name, app.Name)
	s.Stop()

	return nil
}

func runDetachPostgresCluster(ctx *cmdctx.CmdContext) error {
	postgresAppName, _ := ctx.Config.GetString("postgres-app")
	appName := ctx.AppName

	s := spinner.New(spinner.CharSets[11], 100*time.Millisecond)
	s.Writer = os.Stderr
	s.Prefix = "Detaching..."
	s.Start()

	err := ctx.Client.API().DetachPostgresCluster(postgresAppName, appName)

	if err != nil {
		return err
	}

	s.FinalMSG = fmt.Sprintf("Postgres cluster %s is now detached from %s\n", postgresAppName, appName)
	s.Stop()

	return nil
}

func runListPostgresDatabases(ctx *cmdctx.CmdContext) error {
	databases, err := ctx.Client.API().ListPostgresDatabases(ctx.AppName)
	if err != nil {
		return err
	}

	if ctx.OutputJSON() {
		ctx.WriteJSON(databases)
		return nil
	}

	table := helpers.MakeSimpleTable(ctx.Out, []string{"Name", "Users"})

	for _, database := range databases {
		table.Append([]string{database.Name, strings.Join(database.Users, ",")})
	}

	table.Render()

	return nil
}

func runListPostgresUsers(ctx *cmdctx.CmdContext) error {
	users, err := ctx.Client.API().ListPostgresUsers(ctx.AppName)
	if err != nil {
		return err
	}

	if ctx.OutputJSON() {
		ctx.WriteJSON(users)
		return nil
	}

	table := helpers.MakeSimpleTable(ctx.Out, []string{"Username", "Superuser", "Databases"})

	for _, user := range users {
		table.Append([]string{user.Username, strconv.FormatBool(user.IsSuperuser), strings.Join(user.Databases, ",")})
	}

	table.Render()

	return nil
}
