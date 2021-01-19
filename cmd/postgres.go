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

	listStrings := docstrings.Get("postgres.list")
	listCmd := BuildCommandKS(cmd, runPostgresList, listStrings, os.Stdout, requireSession)
	listCmd.Args = cobra.MaximumNArgs(1)

	createStrings := docstrings.Get("postgres.create")
	createCmd := BuildCommandKS(cmd, runCreatePostgresCluster, createStrings, os.Stdout, requireSession)
	createCmd.AddStringFlag(StringFlagOpts{Name: "organization"})
	createCmd.AddStringFlag(StringFlagOpts{Name: "name"})

	attachStrngs := docstrings.Get("postgres.attach")
	attachCmd := BuildCommandKS(cmd, runAttachPostgresCluster, attachStrngs, os.Stdout, requireSession, requireAppName)
	attachCmd.AddStringFlag(StringFlagOpts{Name: "postgres-app"})
	attachCmd.AddStringFlag(StringFlagOpts{Name: "database-name"})
	attachCmd.AddStringFlag(StringFlagOpts{Name: "variable-name"})

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
	orgSlug, _ := ctx.Config.GetString("organization")
	org, err := selectOrganization(ctx.Client.API(), orgSlug)
	if err != nil {
		return err
	}

	name, _ := ctx.Config.GetString("name")
	if name == "" {
		return errors.New("name is required")
	}

	fmt.Fprintf(ctx.Out, "Creating postgres cluster %s in organization %s, this will take a minute...\n", name, org.Slug)

	td, err := ctx.Client.API().CreatePostgresCluster(org.ID, name)
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

// func runPostgresClusterAttach(ctx *cmdctx.CmdContext) error {
// 	name, _ := ctx.Config.GetString("name")
// 	if name == "" {
// 		return errors.New("name is required")
// 	}

// 	fmt.Fprintf(ctx.Out, "Creating postgres cluster %s in organization %s, this will take a minute...\n", name, org.Slug)

// 	return nil
// }
