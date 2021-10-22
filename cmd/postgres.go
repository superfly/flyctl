package cmd

import (
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
	"github.com/superfly/flyctl/internal/client"
)

func newPostgresCommand(client *client.Client) *Command {
	domainsStrings := docstrings.Get("postgres")
	cmd := BuildCommandKS(nil, nil, domainsStrings, client, requireSession)
	cmd.Aliases = []string{"pg"}

	listStrings := docstrings.Get("postgres.list")
	listCmd := BuildCommandKS(cmd, runPostgresList, listStrings, client, requireSession)
	listCmd.Args = cobra.MaximumNArgs(1)

	createStrings := docstrings.Get("postgres.create")
	createCmd := BuildCommandKS(cmd, runCreatePostgresCluster, createStrings, client, requireSession)
	createCmd.AddStringFlag(StringFlagOpts{Name: "organization", Description: "the organization that will own the app"})
	createCmd.AddStringFlag(StringFlagOpts{Name: "name", Description: "the name of the new app"})
	createCmd.AddStringFlag(StringFlagOpts{Name: "region", Description: "the region to launch the new app in"})
	createCmd.AddStringFlag(StringFlagOpts{Name: "password", Description: "the superuser password. one will be generated for you if you leave this blank"})
	createCmd.AddStringFlag(StringFlagOpts{Name: "volume-size", Description: "the size in GB for volumes"})
	createCmd.AddStringFlag(StringFlagOpts{Name: "vm-size", Description: "the size of the VM"})
	createCmd.AddStringFlag(StringFlagOpts{Name: "image-ref", Hidden: true})
	createCmd.AddStringFlag(StringFlagOpts{Name: "snapshot-id", Description: "Creates the volume with the contents of the snapshot"})

	attachStrngs := docstrings.Get("postgres.attach")
	attachCmd := BuildCommandKS(cmd, runAttachPostgresCluster, attachStrngs, client, requireSession, requireAppName)
	attachCmd.AddStringFlag(StringFlagOpts{Name: "postgres-app", Description: "the postgres cluster to attach to the app"})
	attachCmd.AddStringFlag(StringFlagOpts{Name: "database-name", Description: "database to use, defaults to a new database with the same name as the app"})
	attachCmd.AddStringFlag(StringFlagOpts{Name: "variable-name", Description: "the env variable name that will be added to the app. Defaults to DATABASE_URL"})

	detachStrngs := docstrings.Get("postgres.detach")
	detachCmd := BuildCommandKS(cmd, runDetachPostgresCluster, detachStrngs, client, requireSession, requireAppName)
	detachCmd.AddStringFlag(StringFlagOpts{Name: "postgres-app", Description: "the postgres cluster to detach from the app"})

	dbStrings := docstrings.Get("postgres.db")
	dbCmd := BuildCommandKS(cmd, nil, dbStrings, client, requireSession)

	listDBStrings := docstrings.Get("postgres.db.list")
	listDBCmd := BuildCommandKS(dbCmd, runListPostgresDatabases, listDBStrings, client, requireSession, requireAppNameAsArg)
	listDBCmd.Args = cobra.ExactArgs(1)

	usersStrings := docstrings.Get("postgres.users")
	usersCmd := BuildCommandKS(cmd, nil, usersStrings, client, requireSession)

	usersListStrings := docstrings.Get("postgres.users.list")
	usersListCmd := BuildCommandKS(usersCmd, runListPostgresUsers, usersListStrings, client, requireSession, requireAppNameAsArg)
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

func runCreatePostgresCluster(cmdCtx *cmdctx.CmdContext) error {
	ctx := createCancellableContext()
	name := cmdCtx.Config.GetString("name")
	if name == "" {
		n, err := inputAppName("", false)
		if err != nil {
			return err
		}
		name = n
	}

	orgSlug := cmdCtx.Config.GetString("organization")
	org, err := selectOrganization(ctx, cmdCtx.Client.API(), orgSlug, nil)
	if err != nil {
		return err
	}

	regionCode := cmdCtx.Config.GetString("region")
	region, err := selectRegion(ctx, cmdCtx.Client.API(), regionCode)
	if err != nil {
		return err
	}

	vmSizeName := cmdCtx.Config.GetString("vm-size")
	vmSize, err := selectVMSize(ctx, cmdCtx.Client.API(), vmSizeName)
	if err != nil {
		return err
	}

	volumeSize := cmdCtx.Config.GetInt("volume-size")
	if volumeSize == 0 {
		s, err := volumeSizeInput(cmdCtx.Client.API(), 10)
		if err != nil {
			return err
		}
		volumeSize = s
	}

	input := api.CreatePostgresClusterInput{
		OrganizationID: org.ID,
		Name:           name,
		Region:         api.StringPointer(region.Code),
		VMSize:         api.StringPointer(vmSize.Name),
		VolumeSizeGB:   api.IntPointer(volumeSize),
	}

	if imageRef := cmdCtx.Config.GetString("image-ref"); imageRef != "" {
		input.ImageRef = api.StringPointer(imageRef)
	}

	if password := cmdCtx.Config.GetString("password"); password != "" {
		input.Password = api.StringPointer(password)
	}

	snapshot := cmdCtx.Config.GetString("snapshot-id")
	if snapshot != "" {
		input.SnapshotID = api.StringPointer(snapshot)
	}

	fmt.Fprintf(cmdCtx.Out, "Creating postgres cluster %s in organization %s\n", name, org.Slug)

	s := spinner.New(spinner.CharSets[11], 100*time.Millisecond)
	s.Writer = os.Stderr
	s.Prefix = "Launching..."
	s.Start()

	payload, err := cmdCtx.Client.API().CreatePostgresCluster(input)
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

	cancelCtx := createCancellableContext()
	cmdCtx.AppName = payload.App.Name
	err = watchDeployment(cancelCtx, cmdCtx)

	if isCancelledError(err) {
		err = nil
	}

	if err == nil {
		fmt.Println()
		fmt.Println(aurora.Bold("Connect to postgres"))
		fmt.Printf("Any app within the %s organization can connect to postgres using the above credentials and the hostname \"%s.internal.\"\n", org.Slug, payload.App.Name)
		fmt.Printf("For example: postgres://%s:%s@%s.internal:%d\n", payload.Username, payload.Password, payload.App.Name, 5432)

		fmt.Println()
		fmt.Println("See the postgres docs for more information on next steps, managing postgres, connecting from outside fly:  https://fly.io/docs/reference/postgres/")
	}

	return err
}

func runAttachPostgresCluster(ctx *cmdctx.CmdContext) error {
	postgresAppName := ctx.Config.GetString("postgres-app")
	appName := ctx.AppName

	input := api.AttachPostgresClusterInput{
		AppID:                appName,
		PostgresClusterAppID: postgresAppName,
	}

	if dbName := ctx.Config.GetString("database-name"); dbName != "" {
		input.DatabaseName = api.StringPointer(dbName)
	}
	if varName := ctx.Config.GetString("variable-name"); varName != "" {
		input.VariableName = api.StringPointer(varName)
	}

	s := spinner.New(spinner.CharSets[11], 100*time.Millisecond)
	s.Writer = os.Stderr
	s.Prefix = "Attaching..."
	s.Start()

	payload, err := ctx.Client.API().AttachPostgresCluster(input)

	if err != nil {
		return err
	}
	s.Stop()

	fmt.Printf("Postgres cluster %s is now attached to %s\n", payload.PostgresClusterApp.Name, payload.App.Name)
	fmt.Printf("The following secret was added to %s:\n  %s=%s\n", payload.App.Name, payload.EnvironmentVariableName, payload.ConnectionString)

	return nil
}

func runDetachPostgresCluster(ctx *cmdctx.CmdContext) error {
	postgresAppName := ctx.Config.GetString("postgres-app")
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
