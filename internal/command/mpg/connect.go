package mpg

import (
	"context"
	"fmt"
	"os/exec"

	"github.com/logrusorgru/aurora"
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/prompt"
	"github.com/superfly/flyctl/internal/uiex"
	"github.com/superfly/flyctl/internal/uiexutil"
	"github.com/superfly/flyctl/iostreams"
	"github.com/superfly/flyctl/proxy"
)

func newConnect() (cmd *cobra.Command) {
	const (
		long = `Connect to a MPG database using psql`

		short = long
		usage = "connect <CLUSTER ID>"
	)

	cmd = command.New(usage, short, long, runConnect, command.RequireSession)

	flag.Add(cmd,
		flag.String{
			Name:        "database",
			Shorthand:   "d",
			Description: "The database to connect to",
		},
		flag.String{
			Name:        "username",
			Shorthand:   "u",
			Description: "The username to connect as",
		},
	)
	cmd.Args = cobra.MaximumNArgs(1)

	return cmd
}

func runConnect(ctx context.Context) (err error) {
	// Check token compatibility early
	if err := validateMPGTokenCompatibility(ctx); err != nil {
		return err
	}

	io := iostreams.FromContext(ctx)

	localProxyPort := "16380"

	// Get cluster once (will prompt if needed)
	clusterID := flag.FirstArg(ctx)
	var cluster *uiex.ManagedCluster
	var orgSlug string

	if clusterID != "" {
		// If cluster ID is provided, fetch directly without prompting for org
		uiexClient := uiexutil.ClientFromContext(ctx)
		response, err := uiexClient.GetManagedClusterById(ctx, clusterID)
		if err != nil {
			return fmt.Errorf("failed retrieving cluster %s: %w", clusterID, err)
		}
		cluster = &response.Data
		orgSlug = cluster.Organization.Slug
	} else {
		// Otherwise, prompt for org/cluster selection
		var err error
		cluster, orgSlug, err = ClusterFromArgOrSelect(ctx, clusterID, "")
		if err != nil {
			return err
		}
	}

	// Username selection: flag > prompt (if interactive) > empty (use default credentials)
	username := flag.GetString(ctx, "username")
	if username == "" && io.IsInteractive() {
		// Prompt for user selection
		uiexClient := uiexutil.ClientFromContext(ctx)
		usersResponse, err := uiexClient.ListUsers(ctx, cluster.Id)
		if err != nil {
			return fmt.Errorf("failed to list users: %w", err)
		}

		if len(usersResponse.Data) > 0 {
			var userOptions []string
			for _, user := range usersResponse.Data {
				userOptions = append(userOptions, fmt.Sprintf("%s [%s]", user.Name, user.Role))
			}

			var userIndex int
			err = prompt.Select(ctx, &userIndex, "Select user:", "", userOptions...)
			if err != nil {
				return err
			}

			username = usersResponse.Data[userIndex].Name
		}
		// If no users found, username remains empty and will use default credentials
	}

	// Database selection priority: flag > prompt result (if interactive) > credentials.DBName
	// We'll get credentials from getMpgProxyParams, but need to prompt for database first if needed
	var db string
	if database := flag.GetString(ctx, "database"); database != "" {
		db = database
	} else if io.IsInteractive() {
		// Prompt for database selection
		uiexClient := uiexutil.ClientFromContext(ctx)
		databasesResponse, err := uiexClient.ListDatabases(ctx, cluster.Id)
		if err != nil {
			return fmt.Errorf("failed to list databases: %w", err)
		}

		if len(databasesResponse.Data) > 0 {
			var dbOptions []string
			for _, database := range databasesResponse.Data {
				dbOptions = append(dbOptions, database.Name)
			}

			var dbIndex int
			err = prompt.Select(ctx, &dbIndex, "Select database:", "", dbOptions...)
			if err != nil {
				return err
			}

			db = databasesResponse.Data[dbIndex].Name
		}
	}

	cluster, params, credentials, err := getMpgProxyParamsWithCluster(ctx, localProxyPort, username, cluster.Id, orgSlug)
	if err != nil {
		return err
	}

	if cluster.Status != "ready" {
		fmt.Fprintf(io.ErrOut, "%s Cluster is not in ready state, currently: %s\n", aurora.Yellow("WARN"), cluster.Status)
	}

	psqlPath, err := exec.LookPath("psql")
	if err != nil {
		fmt.Fprintf(io.Out, "Could not find psql in your $PATH. Install it or point your psql at: %s", "someurl")
		return
	}

	err = proxy.Start(ctx, params)
	if err != nil {
		return err
	}

	user := credentials.User
	password := credentials.Password

	// Use selected database or fall back to default from credentials
	if db == "" {
		db = credentials.DBName
	}

	connectUrl := fmt.Sprintf("postgresql://%s:%s@localhost:%s/%s", user, password, localProxyPort, db)
	cmd := exec.CommandContext(ctx, psqlPath, connectUrl)
	cmd.Stdout = io.Out
	cmd.Stderr = io.ErrOut
	cmd.Stdin = io.In

	cmd.Start()
	cmd.Wait()

	return
}
