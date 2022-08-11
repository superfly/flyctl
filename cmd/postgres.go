package cmd

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/AlecAivazis/survey/v2"
	"github.com/briandowns/spinner"
	"github.com/hashicorp/go-version"
	"github.com/logrusorgru/aurora"
	"github.com/pkg/errors"
	"github.com/superfly/flyctl/agent"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/cmdctx"
	"github.com/superfly/flyctl/flypg"
	"github.com/superfly/flyctl/helpers"
	"github.com/superfly/flyctl/internal/flag"
)

type PostgresConfiguration struct {
	Name               string
	Description        string
	ImageRef           string
	InitialClusterSize int
	VmSize             string
	MemoryMb           int
	DiskGb             int
}

func postgresConfigurations() []PostgresConfiguration {
	return []PostgresConfiguration{
		{
			Description:        "Development - Single node, 1x shared CPU, 256MB RAM, 1GB disk",
			DiskGb:             1,
			ImageRef:           "flyio/postgres",
			InitialClusterSize: 1,
			MemoryMb:           256,
			VmSize:             "shared-cpu-1x",
		},
		{
			Description:        "Production - Highly available, 1x shared CPU, 256MB RAM, 10GB disk",
			DiskGb:             10,
			ImageRef:           "flyio/postgres",
			InitialClusterSize: 2,
			MemoryMb:           256,
			VmSize:             "shared-cpu-1x",
		},
		{
			Description:        "Production - Highly available, 1x Dedicated CPU, 2GB RAM, 50GB disk",
			DiskGb:             50,
			ImageRef:           "flyio/postgres",
			InitialClusterSize: 2,
			MemoryMb:           2048,
			VmSize:             "dedicated-cpu-1x",
		},
		{
			Description:        "Production - Highly available, 2x Dedicated CPU's, 4GB RAM, 100GB disk",
			DiskGb:             100,
			ImageRef:           "flyio/postgres",
			InitialClusterSize: 2,
			MemoryMb:           4096,
			VmSize:             "dedicated-cpu-2x",
		},
		{
			Description:        "Specify custom configuration",
			DiskGb:             0,
			ImageRef:           "flyio/postgres",
			InitialClusterSize: 0,
			MemoryMb:           0,
			VmSize:             "",
		},
	}
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
	org, err := selectOrganization(ctx, cmdCtx.Client.API(), orgSlug)
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
		ImageRef:       api.StringPointer("flyio/postgres"),
	}

	volumeSize := cmdCtx.Config.GetInt("volume-size")
	initialClusterSize := cmdCtx.Config.GetInt("initial-cluster-size")
	vmSizeName := cmdCtx.Config.GetString("vm-size")

	customConfig := volumeSize != 0 || vmSizeName != "" || initialClusterSize != 0

	var pgConfig *PostgresConfiguration
	var vmSize *api.VMSize

	// If no custom configuration settings have been passed in, prompt user to select
	// from a list of pre-defined configurations or opt into specifying a custom
	// configuration.
	if !customConfig {
		fmt.Println(aurora.Yellow("For pricing information visit: https://fly.io/docs/about/pricing/#postgresql-clusters"))
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
		// Resolve cluster size
		if initialClusterSize == 0 {
			initialClusterSize, err = initialClusterSizeInput(2)
			if err != nil {
				return err
			}
		}
		input.Count = &initialClusterSize

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

		input.Count = api.IntPointer(pgConfig.InitialClusterSize)

		if imageRef := cmdCtx.Config.GetString("image-ref"); imageRef != "" {
			input.ImageRef = api.StringPointer(imageRef)
		} else {
			input.ImageRef = &pgConfig.ImageRef
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
	err = watchDeployment(cancelCtx, cmdCtx, "")

	if isCancelledError(err) {
		err = nil
	}

	if err == nil {
		fmt.Println()
		fmt.Println(aurora.Bold("Connect to postgres"))
		fmt.Printf("Any app within the %s organization can connect to postgres using the above credentials and the hostname \"%s.internal.\"\n", org, payload.App.Name)
		fmt.Printf("For example: postgres://%s:%s@%s.internal:%d\n", payload.Username, payload.Password, payload.App.Name, 5432)

		fmt.Println()
		fmt.Println("Now you've setup postgres, here's what you need to understand: https://fly.io/docs/reference/postgres-whats-next/")
	}

	return payload, err
}

func runAttachPostgresCluster(cmdCtx *cmdctx.CmdContext) error {
	// Minimum image version requirements

	MinPostgresHaVersion := "0.0.19"

	ctx := cmdCtx.Command.Context()

	postgresAppName := cmdCtx.Config.GetString("postgres-app")
	appName := cmdCtx.AppName

	dbName := cmdCtx.Config.GetString("database-name")
	if dbName == "" {
		dbName = appName
	}
	dbName = strings.ToLower(strings.ReplaceAll(dbName, "-", "_"))

	varName := cmdCtx.Config.GetString("variable-name")
	if varName == "" {
		varName = "DATABASE_URL"
	}

	dbUser := cmdCtx.Config.GetString("database-user")
	if dbUser == "" {
		dbUser = appName
	}
	dbUser = strings.ToLower(strings.ReplaceAll(dbUser, "-", "_"))

	input := api.AttachPostgresClusterInput{
		AppID:                appName,
		PostgresClusterAppID: postgresAppName,
		ManualEntry:          true,
		DatabaseName:         api.StringPointer(dbName),
		DatabaseUser:         api.StringPointer(dbUser),
		VariableName:         api.StringPointer(varName),
	}

	client := cmdCtx.Client.API()

	app, err := client.GetAppBasic(ctx, appName)
	if err != nil {
		return fmt.Errorf("get app: %w", err)
	}

	pgApp, err := client.GetAppPostgres(ctx, postgresAppName)
	if err != nil {
		return fmt.Errorf("get app: %w", err)
	}

	if err := hasRequiredVersion(pgApp, MinPostgresHaVersion, ""); err != nil {
		return err
	}

	agentclient, err := agent.Establish(ctx, cmdCtx.Client.API())
	if err != nil {
		return errors.Wrap(err, "can't establish agent")
	}

	dialer, err := agentclient.Dialer(ctx, pgApp.Organization.Slug)
	if err != nil {
		return fmt.Errorf("ssh: can't build tunnel for %s: %s", pgApp.Organization.Slug, err)
	}

	pgclient := flypg.New(postgresAppName, dialer)

	secrets, err := client.GetAppSecrets(ctx, appName)
	if err != nil {
		return err
	}
	for _, secret := range secrets {
		if secret.Name == *input.VariableName {
			return fmt.Errorf("Consumer app %q already contains a secret named %s.", appName, *input.VariableName)
		}
	}
	// Check to see if database exists
	dbExists, err := pgclient.DatabaseExists(ctx, *input.DatabaseName)
	if err != nil {
		return err
	}
	if dbExists && !flag.GetBool(ctx, "force") {
		confirm := false
		prompt := &survey.Confirm{
			Message: fmt.Sprintf("Database %q already exists. Continue with the attachment process?", *input.DatabaseName),
		}
		err = survey.AskOne(prompt, &confirm)
		if err != nil {
			return err
		}

		if !confirm {
			return nil
		}
	}

	// Check to see if user exists
	usrExists, err := pgclient.UserExists(ctx, *input.DatabaseUser)
	if err != nil {
		return err
	}
	if usrExists {
		return fmt.Errorf("Database user %q already exists. Please specify a new database user via --database-user", *input.DatabaseUser)
	}

	// Create attachment
	_, err = client.AttachPostgresCluster(ctx, input)
	if err != nil {
		return err
	}

	// Create database if it doesn't already exist
	if !dbExists {
		err := pgclient.CreateDatabase(ctx, *input.DatabaseName)
		if err != nil {
			if flypg.ErrorStatus(err) >= 500 {
				return err
			}
			return fmt.Errorf("failed executing database-create: %w", err)
		}

	}

	// Create user
	pwd, err := helpers.RandString(15)
	if err != nil {
		return err
	}

	if err := pgclient.CreateUser(ctx, *input.DatabaseUser, pwd, true); err != nil {
		return fmt.Errorf("failed executing create-user: %w", err)
	}

	connectionString := fmt.Sprintf("postgres://%s:%s@top2.nearest.of.%s.internal:5432/%s", *input.DatabaseUser, pwd, postgresAppName, *input.DatabaseName)
	s := map[string]string{}
	s[*input.VariableName] = connectionString

	_, err = client.SetSecrets(ctx, appName, s)
	if err != nil {
		return err
	}

	fmt.Printf("\nPostgres cluster %s is now attached to %s\n", postgresAppName, app.Name)
	fmt.Printf("The following secret was added to %s:\n  %s=%s\n", app.Name, *input.VariableName, connectionString)

	return nil
}

func hasRequiredVersion(app *api.AppPostgres, cluster, standalone string) error {
	// Validate image version to ensure it's compatible with this feature.
	if app.ImageDetails.Version == "" || app.ImageDetails.Version == "unknown" {
		return fmt.Errorf("Command is not compatible with this image.")
	}

	imageVersionStr := app.ImageDetails.Version[1:]
	imageVersion, err := version.NewVersion(imageVersionStr)
	if err != nil {
		return err
	}

	// Specify compatible versions per repo.
	requiredVersion := &version.Version{}
	if app.ImageDetails.Repository == "flyio/postgres-standalone" {
		requiredVersion, err = version.NewVersion(standalone)
		if err != nil {
			return err
		}
	}
	if app.ImageDetails.Repository == "flyio/postgres" {
		requiredVersion, err = version.NewVersion(cluster)
		if err != nil {
			return err
		}
	}

	if requiredVersion == nil {
		return fmt.Errorf("Unable to resolve image version...")
	}

	if imageVersion.LessThan(requiredVersion) {
		return fmt.Errorf(
			"Image version is not compatible. (Current: %s, Required: >= %s)\n"+
				"Please run 'flyctl image show' and update to the latest available version.",
			imageVersion, requiredVersion.String())
	}

	return nil
}
