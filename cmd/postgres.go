package cmd

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/AlecAivazis/survey/v2"
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

type PostgresClusterOption struct {
	Name     string
	ImageRef string
	Count    int
}
type PostgresConfiguration struct {
	Name             string
	Description      string
	VmSize           string
	MemoryMb         int
	DiskGb           int
	ClusteringOption PostgresClusterOption
}

func postgresConfigurations() []PostgresConfiguration {
	return []PostgresConfiguration{
		{
			Description:      "Development - Single node, 1x shared CPU, 256MB RAM, 10GB disk",
			VmSize:           "shared-cpu-1x",
			MemoryMb:         256,
			DiskGb:           10,
			ClusteringOption: standalonePostgres(),
		},
		{
			Description:      "Production - Highly available, 1x shared CPU, 256MB RAM, 10GB disk",
			VmSize:           "shared-cpu-1x",
			MemoryMb:         256,
			DiskGb:           10,
			ClusteringOption: highlyAvailablePostgres(),
		},
		{
			Description:      "Production - Highly available, 1x Dedicated CPU, 2GB RAM, 50GB disk",
			VmSize:           "dedicated-cpu-1x",
			MemoryMb:         2048,
			DiskGb:           50,
			ClusteringOption: highlyAvailablePostgres(),
		},
		{
			Description:      "Production - Highly available, 2x Dedicated CPU's, 4GB RAM, 100GB disk",
			VmSize:           "dedicated-cpu-2x",
			MemoryMb:         4096,
			DiskGb:           100,
			ClusteringOption: highlyAvailablePostgres(),
		},
		{
			Description:      "Production - Highly available, 4x Dedicated CPU's, 8GB RAM, 200GB disk",
			VmSize:           "dedicated-cpu-4x",
			MemoryMb:         8192,
			DiskGb:           200,
			ClusteringOption: highlyAvailablePostgres(),
		},
		{
			Description: "Specify custom configuration",
			VmSize:      "",
			MemoryMb:    0,
			DiskGb:      0,
		},
	}
}

func standalonePostgres() PostgresClusterOption {
	return PostgresClusterOption{
		Name:     "Development (Single node)",
		ImageRef: "flyio/postgres-standalone",
		Count:    1,
	}
}

func highlyAvailablePostgres() PostgresClusterOption {
	return PostgresClusterOption{
		Name:     "Production (Highly available)",
		ImageRef: "flyio/postgres",
		Count:    2,
	}
}

func postgresClusteringOptions() []PostgresClusterOption {
	return []PostgresClusterOption{
		standalonePostgres(),
		highlyAvailablePostgres(),
	}
}

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
	apps, err := ctx.Client.API().GetApps(context.Background(), api.StringPointer("postgres_cluster"))
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
	ctx := cmdCtx.Command.Context()

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
	var region *api.Region
	region, err = selectRegion(ctx, cmdCtx.Client.API(), regionCode)
	if err != nil {
		return err
	}

	input := api.CreatePostgresClusterInput{
		OrganizationID: org.ID,
		Name:           name,
		Region:         api.StringPointer(region.Code),
	}

	customConfig := false

	volumeSize := cmdCtx.Config.GetInt("volume-size")
	vmSizeName := cmdCtx.Config.GetString("vm-size")

	if volumeSize != 0 || vmSizeName != "" {
		customConfig = true
	}

	var pgConfig *PostgresConfiguration
	var vmSize *api.VMSize

	// If no custom configuration settings have been passed in, prompt user to select
	// from a list of pre-defined configurations or opt into specifying a custom
	// configuration.
	if !customConfig {
		selectedCfg := 0
		options := []string{}
		for _, cfg := range postgresConfigurations() {
			options = append(options, cfg.Description)
		}
		prompt := &survey.Select{
			Message:  "Select configuration:",
			Options:  options,
			PageSize: len(postgresConfigurations()),
		}
		if err := survey.AskOne(prompt, &selectedCfg); err != nil {
			return err
		}
		pgConfig = &postgresConfigurations()[selectedCfg]

		if pgConfig.VmSize == "" {
			// User has opted into choosing a custom configuration.
			customConfig = true
		}
	}

	if customConfig {
		selected := 0
		options := []string{}
		for _, opt := range postgresClusteringOptions() {
			options = append(options, opt.Name)
		}
		prompt := &survey.Select{
			Message:  "Select configuration:",
			Options:  options,
			PageSize: 2,
		}
		if err := survey.AskOne(prompt, &selected); err != nil {
			return err
		}
		option := postgresClusteringOptions()[selected]

		input.Count = &option.Count
		input.ImageRef = &option.ImageRef

		// Resolve VM size
		vmSize, err = selectVMSize(ctx, cmdCtx.Client.API(), vmSizeName)
		if err != nil {
			return err
		}
		input.VMSize = api.StringPointer(vmSize.Name)

		// Resolve volume size
		if volumeSize == 0 {
			volumeSize, err = volumeSizeInput(10)
			if err != nil {
				return err
			}
		}
		input.VolumeSizeGB = api.IntPointer(volumeSize)

	} else {
		// Resolve configuration from pre-defined configuration.
		vmSize, err = selectVMSize(ctx, cmdCtx.Client.API(), pgConfig.VmSize)
		if err != nil {
			return err
		}
		input.VMSize = api.StringPointer(vmSize.Name)
		input.VolumeSizeGB = api.IntPointer(pgConfig.DiskGb)
		input.Count = api.IntPointer(pgConfig.ClusteringOption.Count)

		if imageRef := cmdCtx.Config.GetString("image-ref"); imageRef != "" {
			input.ImageRef = api.StringPointer(imageRef)
		} else {
			input.ImageRef = &pgConfig.ClusteringOption.ImageRef
		}
	}

	if password := cmdCtx.Config.GetString("password"); password != "" {
		input.Password = api.StringPointer(password)
	}

	snapshot := cmdCtx.Config.GetString("snapshot-id")
	if snapshot != "" {
		input.SnapshotID = api.StringPointer(snapshot)
	}

	fmt.Fprintf(cmdCtx.Out, "Creating postgres cluster %s in organization %s\n", name, org.Slug)

	_, err = runApiCreatePostgresCluster(cmdCtx, org.Slug, &input)

	return err
}

func runApiCreatePostgresCluster(cmdCtx *cmdctx.CmdContext, org string, input *api.CreatePostgresClusterInput) (*api.CreatePostgresClusterPayload, error) {
	s := spinner.New(spinner.CharSets[11], 100*time.Millisecond)
	s.Writer = os.Stderr
	s.Prefix = "Launching..."
	s.Start()

	payload, err := cmdCtx.Client.API().CreatePostgresCluster(cmdCtx.Command.Context(), *input)
	if err != nil {
		return nil, err
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

	cancelCtx := cmdCtx.Command.Context()
	cmdCtx.AppName = payload.App.Name
	err = watchDeployment(cancelCtx, cmdCtx)

	if isCancelledError(err) {
		err = nil
	}

	if err == nil {
		fmt.Println()
		fmt.Println(aurora.Bold("Connect to postgres"))
		fmt.Printf("Any app within the %s organization can connect to postgres using the above credentials and the hostname \"%s.internal.\"\n", org, payload.App.Name)
		fmt.Printf("For example: postgres://%s:%s@%s.internal:%d\n", payload.Username, payload.Password, payload.App.Name, 5432)

		fmt.Println()
		fmt.Println("See the postgres docs for more information on next steps, managing postgres, connecting from outside fly:  https://fly.io/docs/reference/postgres/")
	}

	return payload, err
}

func runAttachPostgresCluster(cmdCtx *cmdctx.CmdContext) error {
	ctx := cmdCtx.Command.Context()

	postgresAppName := cmdCtx.Config.GetString("postgres-app")
	appName := cmdCtx.AppName

	input := api.AttachPostgresClusterInput{
		AppID:                appName,
		PostgresClusterAppID: postgresAppName,
	}

	if dbName := cmdCtx.Config.GetString("database-name"); dbName != "" {
		input.DatabaseName = api.StringPointer(dbName)
	}
	if varName := cmdCtx.Config.GetString("variable-name"); varName != "" {
		input.VariableName = api.StringPointer(varName)
	}

	s := spinner.New(spinner.CharSets[11], 100*time.Millisecond)
	s.Writer = os.Stderr
	s.Prefix = "Attaching..."
	s.Start()

	payload, err := cmdCtx.Client.API().AttachPostgresCluster(ctx, input)

	if err != nil {
		return err
	}
	s.Stop()

	fmt.Printf("Postgres cluster %s is now attached to %s\n", payload.PostgresClusterApp.Name, payload.App.Name)
	fmt.Printf("The following secret was added to %s:\n  %s=%s\n", payload.App.Name, payload.EnvironmentVariableName, payload.ConnectionString)

	return nil
}

func runDetachPostgresCluster(cmdCtx *cmdctx.CmdContext) error {
	ctx := cmdCtx.Command.Context()

	postgresAppName := cmdCtx.Config.GetString("postgres-app")
	appName := cmdCtx.AppName

	s := spinner.New(spinner.CharSets[11], 100*time.Millisecond)
	s.Writer = os.Stderr
	s.Prefix = "Detaching..."
	s.Start()

	err := cmdCtx.Client.API().DetachPostgresCluster(ctx, postgresAppName, appName)

	if err != nil {
		return err
	}

	s.FinalMSG = fmt.Sprintf("Postgres cluster %s is now detached from %s\n", postgresAppName, appName)
	s.Stop()

	return nil
}

func runListPostgresDatabases(cmdCtx *cmdctx.CmdContext) error {
	ctx := cmdCtx.Command.Context()

	databases, err := cmdCtx.Client.API().ListPostgresDatabases(ctx, cmdCtx.AppName)
	if err != nil {
		return err
	}

	if cmdCtx.OutputJSON() {
		cmdCtx.WriteJSON(databases)
		return nil
	}

	table := helpers.MakeSimpleTable(cmdCtx.Out, []string{"Name", "Users"})

	for _, database := range databases {
		table.Append([]string{database.Name, strings.Join(database.Users, ",")})
	}

	table.Render()

	return nil
}

func runListPostgresUsers(cmdCtx *cmdctx.CmdContext) error {
	ctx := cmdCtx.Command.Context()

	users, err := cmdCtx.Client.API().ListPostgresUsers(ctx, cmdCtx.AppName)
	if err != nil {
		return err
	}

	if cmdCtx.OutputJSON() {
		cmdCtx.WriteJSON(users)
		return nil
	}

	table := helpers.MakeSimpleTable(cmdCtx.Out, []string{"Username", "Superuser", "Databases"})

	for _, user := range users {
		table.Append([]string{user.Username, strconv.FormatBool(user.IsSuperuser), strings.Join(user.Databases, ",")})
	}

	table.Render()

	return nil
}
