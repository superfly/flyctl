package cmdv2

import (
	"context"
	"fmt"
	"io"

	"github.com/logrusorgru/aurora"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/mpgutil"
	"github.com/superfly/flyctl/internal/prompt"
	mpgv2 "github.com/superfly/flyctl/internal/uiex/mpg/v2"
	"github.com/superfly/flyctl/iostreams"
	"github.com/superfly/flyctl/proxy"
)

func RunConnect(ctx context.Context, clusterID string, resolvedOrgSlug string, proxyPort string, psqlPath string) (err error) {
	io := iostreams.FromContext(ctx)
	options := mpgutil.PsqlOptionsFromContext(ctx)

	localProxyPort := proxyPort

	// Username selection: flag > prompt (if interactive) > empty (use default credentials)
	username := flag.GetString(ctx, "username")
	if username == "" && io.IsInteractive() && !options.Scripted() {
		// Prompt for user selection
		mpgClient := mpgv2.ClientFromContext(ctx)
		usersResponse, err := mpgClient.ListUsers(ctx, clusterID)
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
	var db string
	if database := flag.GetString(ctx, "database"); database != "" {
		db = database
	} else if io.IsInteractive() && !options.Scripted() {
		// Prompt for database selection
		mpgClient := mpgv2.ClientFromContext(ctx)
		databasesResponse, err := mpgClient.ListDatabases(ctx, clusterID)
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

	cluster, params, credentials, err := GetMpgConnectParams(ctx, localProxyPort, username, clusterID, resolvedOrgSlug)
	if err != nil {
		return err
	}

	maybeWarnNotReady(io.ErrOut, cluster)

	// We want to handle cancels ourselves, since they can pass through
	// as query cancellations to psql without killing the proxy.
	proxyCtx, proxyCancel := context.WithCancelCause(context.WithoutCancel(mpgutil.PsqlProxyContext(ctx, options)))
	defer proxyCancel(fmt.Errorf("Postgres proxy stopped: %w", context.Canceled))

	err = proxy.Start(proxyCtx, params)
	if err != nil {
		return err
	}

	connectUrl := buildConnectURL(credentials, db, localProxyPort)

	return mpgutil.RunPsql(ctx, psqlPath, connectUrl, options)
}

// buildConnectURL prefers the selected database over the credential default.
func buildConnectURL(credentials *mpgv2.GetClusterCredentialsResponse, db string, localProxyPort string) string {
	if db == "" {
		db = credentials.DBName
	}

	return fmt.Sprintf("postgresql://%s:%s@localhost:%s/%s", credentials.User, credentials.Password, localProxyPort, db)
}

// maybeWarnNotReady warns when a cluster is not in ready state.
func maybeWarnNotReady(errOut io.Writer, cluster *mpgv2.ManagedCluster) {
	if cluster == nil || cluster.Status == "ready" {
		return
	}

	fmt.Fprintf(errOut, "%s Cluster is not in ready state, currently: %s\n", aurora.Yellow("WARN"), cluster.Status)
}
