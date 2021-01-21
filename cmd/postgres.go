package cmd

import (
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/cmd/presenters"
	"github.com/superfly/flyctl/cmdctx"
	"github.com/superfly/flyctl/docstrings"
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

	attachStrngs := docstrings.Get("postgres.attach")
	attachCmd := BuildCommandKS(cmd, runAttachPostgresCluster, attachStrngs, os.Stdout, requireSession, requireAppName)
	attachCmd.AddStringFlag(StringFlagOpts{Name: "postgres-app", Description: "the postgres cluster to attach to the app"})
	attachCmd.AddStringFlag(StringFlagOpts{Name: "database-name", Description: "database to use, defaults to a new database with the same name as the app"})
	attachCmd.AddStringFlag(StringFlagOpts{Name: "variable-name", Description: "the env variable name that will be added to the app. Defaults to DATABASE_URL"})

	detachStrngs := docstrings.Get("postgres.detach")
	detachCmd := BuildCommandKS(cmd, runDetachPostgresCluster, detachStrngs, os.Stdout, requireSession, requireAppName)
	detachCmd.AddStringFlag(StringFlagOpts{Name: "postgres-app", Description: "the postgres cluster to detach from the app"})

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

	fmt.Fprintf(ctx.Out, "Creating postgres cluster %s in organization %s, this will take a minute...\n", name, org.Slug)

	td, err := ctx.Client.API().CreatePostgresCluster(org.ID, name, region)
	if err != nil {
		return err
	}

	for {
		td, err = ctx.Client.API().GetTemplateDeployment(td.ID)
		if err != nil {
			return err
		}

		if td.Status == "failed" {
			fmt.Fprintf(ctx.Out, "Failed to create postgres cluster, please try again\n")
			break
		} else if td.Status == "completed" {
			app := td.Apps.Nodes[0]
			if app.Status == "running" && app.State == "DEPLOYED" {
				fmt.Fprintf(ctx.Out, "Postgres cluster created: %s\n", td.Apps.Nodes[0].Name)
				break
			}

		}

		fmt.Println("launching...")

		time.Sleep(1 * time.Second)
	}

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

	app, postgresApp, err := ctx.Client.API().AttachPostgresCluster(input)

	if err != nil {
		return err
	}

	fmt.Printf("Postgres cluster %s is now attached to %s\n", postgresApp.Name, app.Name)

	return nil
}

func runDetachPostgresCluster(ctx *cmdctx.CmdContext) error {
	postgresAppName, _ := ctx.Config.GetString("postgres-app")
	appName := ctx.AppName

	err := ctx.Client.API().DetachPostgresCluster(postgresAppName, appName)

	if err != nil {
		return err
	}

	fmt.Printf("Postgres cluster %s is now detached from %s\n", postgresAppName, appName)

	return nil
}

// func runPostgresClusterAttach(ctx *cmdctx.CmdContext) error {
// 	name, _ := ctx.Config.GetString("name")
// 	if name == "" {
// 		return errors.New("name is required")
// 	}

// 	fmt.Fprintf(ctx.Out, "Creating postgres cluster %s in organization %s, this will take a minute...\n", name, org.Slug)

// 	return nil
// }
