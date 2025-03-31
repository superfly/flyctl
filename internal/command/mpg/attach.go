package mpg

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/uiex"
	"github.com/superfly/flyctl/internal/uiexutil"
	"github.com/superfly/flyctl/iostreams"
)

type AttachParams struct {
	DbName       string
	AppName      string
	ClusterId    string
	DbUser       string
	VariableName string
}

func newAttach() *cobra.Command {
	const (
		short = "Attach a managed postgres cluster to an app"
		long  = short + "\n"
		usage = "attach <CLUSTER ID>"
	)

	cmd := command.New(usage, short, long, runAttach,
		command.RequireSession,
		command.RequireAppName,
		command.RequireUiex,
	)
	cmd.Args = cobra.ExactArgs(1)

	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
		flag.String{
			Name:        "database-name",
			Description: "The designated database name for this consuming app.",
		},
		flag.String{
			Name:        "database-user",
			Description: "The database user to create. By default, we will use the name of the consuming app.",
		},
		flag.String{
			Name:        "variable-name",
			Default:     "DATABASE_URL",
			Description: "The environment variable name that will be added to the consuming app. ",
		},
		flag.Yes(),
	)

	return cmd
}

func runAttach(ctx context.Context) error {
	var (
		clusterId  = flag.FirstArg(ctx)
		appName    = appconfig.NameFromContext(ctx)
		client     = flyutil.ClientFromContext(ctx)
		uiexClient = uiexutil.ClientFromContext(ctx)
	)

	response, err := uiexClient.GetManagedClusterById(ctx, clusterId)

	if err != nil {
		return fmt.Errorf("failed retrieving cluster %s: %w", clusterId, err)
	}
	cluster := response.Data

	app, err := client.GetAppCompact(ctx, appName)
	if err != nil {
		return fmt.Errorf("failed retrieving app %s: %w", appName, err)
	}

	params := AttachParams{
		AppName:      app.Name,
		ClusterId:    cluster.Id,
		DbName:       flag.GetString(ctx, "database-name"),
		DbUser:       flag.GetString(ctx, "database-user"),
		VariableName: flag.GetString(ctx, "variable-name"),
	}

	return runAttachCluster(ctx, params)
}

func machineAttachCluster(ctx context.Context, params AttachParams, flycast *string) error {
	return runAttachCluster(ctx, params)
}

func runAttachCluster(ctx context.Context, params AttachParams) error {
	var (
		client     = flyutil.ClientFromContext(ctx)
		uiexClient = uiexutil.ClientFromContext(ctx)
		io         = iostreams.FromContext(ctx)

		appName   = params.AppName
		clusterId = params.ClusterId
		dbName    = params.DbName
		dbUser    = params.DbUser
		varName   = params.VariableName
	)

	if dbName == "" {
		dbName = appName
	}

	if dbUser == "" {
		dbUser = appName
	}

	dbUser = strings.ToLower(strings.ReplaceAll(dbUser, "_", "-"))

	if varName == "" {
		varName = "DATABASE_URL"
	}

	dbName = strings.ToLower(strings.ReplaceAll(dbName, "_", "-"))

	input := uiex.CreateUserInput{
		DbName:   dbName,
		UserName: dbUser,
	}

	response, err := uiexClient.CreateUser(ctx, clusterId, input)
	if err != nil {
		return err
	}

	s := map[string]string{}
	s[varName] = response.ConnectionUri

	_, err = client.SetSecrets(ctx, appName, s)
	if err != nil {
		return err
	}

	fmt.Fprintf(io.Out, "\nPostgres cluster %s is being attached to %s\n", clusterId, appName)
	fmt.Fprintf(io.Out, "The following secret was added to %s:\n  %s=%s\n", appName, varName, response.ConnectionUri)

	return nil
}
