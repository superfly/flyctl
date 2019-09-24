package cmd

import (
	"fmt"
	"os"

	"github.com/AlecAivazis/survey/v2"
	"github.com/logrusorgru/aurora"
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/cmd/presenters"
)

func newDatabasesCommand() *Command {
	cmd := &Command{
		Command: &cobra.Command{
			Use:   "dbs",
			Short: "manage databases",
			Long:  "manage databases",
		},
	}

	BuildCommand(cmd, runDatabasesList, "list", "list databases", os.Stdout, true)
	delete := BuildCommand(cmd, runDestroyDatabase, "destroy", "permanently destroy a database", os.Stdout, true)
	delete.Args = cobra.ExactArgs(1)
	delete.AddBoolFlag(BoolFlagOpts{Name: "yes", Shorthand: "y", Description: "accept all confirmations"})

	BuildCommand(cmd, runCreateDatabase, "create", "create database", os.Stdout, true)
	show := BuildCommand(cmd, runDatabaseShow, "show", "show detailed database information", os.Stdout, true)
	show.Args = cobra.ExactArgs(1)

	return cmd
}

func runDatabasesList(ctx *CmdContext) error {
	dbs, err := ctx.FlyClient.GetDatabases()
	if err != nil {
		return err
	}

	return ctx.Render(&presenters.Databases{Databases: dbs})
}

var dbEngines = []string{"cockroachdb", "redis"}

func runCreateDatabase(ctx *CmdContext) error {
	name, _ := ctx.Config.GetString("name")
	if name == "" {
		prompt := &survey.Input{
			Message: "Database Name (leave blank to use an auto-generated name)",
		}
		if err := survey.AskOne(prompt, &name); err != nil {
			if isInterrupt(err) {
				return nil
			}
			return err
		}
	}

	engine, _ := ctx.Config.GetString("engine")
	if engine == "" {
		prompt := &survey.Select{
			Message:  "Select engine:",
			Options:  dbEngines,
			PageSize: 15,
		}
		if err := survey.AskOne(prompt, &engine); err != nil {
			if isInterrupt(err) {
				return nil
			}
			return err
		}
	}

	targetOrgSlug, _ := ctx.Config.GetString("org")
	org, err := selectOrganization(ctx.FlyClient, targetOrgSlug)

	switch {
	case isInterrupt(err):
		return nil
	case err != nil || org == nil:
		return fmt.Errorf("Error setting organization: %s", err)
	}

	db, err := ctx.FlyClient.CreateDatabase(org.ID, name, engine)
	if err != nil {
		return err
	}

	fmt.Println("New database created")

	err = ctx.RenderEx(&presenters.DatabaseInfo{Database: *db}, presenters.Options{HideHeader: true, Vertical: true})
	if err != nil {
		return err
	}

	return nil
}

func runDatabaseShow(ctx *CmdContext) error {
	id := ctx.Args[0]

	db, err := ctx.FlyClient.GetDatabase(id)
	if err != nil {
		return err
	}

	err = ctx.RenderEx(&presenters.DatabaseInfo{Database: *db}, presenters.Options{HideHeader: true, Vertical: true})
	if err != nil {
		return err
	}

	return nil
}

func runDestroyDatabase(ctx *CmdContext) error {
	databaseId := ctx.Args[0]

	db, err := ctx.FlyClient.GetDatabase(databaseId)
	if err != nil {
		return err
	}

	if !ctx.Config.GetBool("yes") {
		fmt.Println(aurora.Red("Destroying a database is not reversible."))

		if !confirm("Destroy database?") {
			return nil
		}
	}

	if _, err := ctx.FlyClient.DestroyDatabase(databaseId); err != nil {
		return err
	}

	fmt.Println("Destroyed database", db.Name)

	return nil
}
