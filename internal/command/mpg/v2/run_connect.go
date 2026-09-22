package cmdv2

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"

	"github.com/logrusorgru/aurora"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/prompt"
	mpgv2 "github.com/superfly/flyctl/internal/uiex/mpg/v2"
	"github.com/superfly/flyctl/iostreams"
	"github.com/superfly/flyctl/proxy"
)

func RunConnect(ctx context.Context, clusterID string, resolvedOrgSlug string, proxyPort string) (err error) {
	io := iostreams.FromContext(ctx)

	localProxyPort := proxyPort

	// Resolve the cluster once, up front, so the interactive pickers below
	// and connectParamsFromCluster agree on whether this cluster only exists in
	// the legacy API (useLegacy) or was resolved through the public
	// Machines API. This mirrors getCluster's own fallback decision instead
	// of introducing a second, independent 404 check.
	response, useLegacy, port, err := getCluster(ctx, clusterID)
	if err != nil {
		return err
	}

	// Username selection: flag > prompt (if interactive) > empty (use default credentials)
	username := flag.GetString(ctx, "username")
	if username == "" && io.IsInteractive() {
		// Prompt for user selection
		users, err := listConnectUsers(ctx, useLegacy, clusterID)
		if err != nil {
			return fmt.Errorf("failed to list users: %w", err)
		}

		if len(users) > 0 {
			var userOptions []string
			for _, user := range users {
				userOptions = append(userOptions, fmt.Sprintf("%s [%s]", user.Name, user.Role))
			}

			var userIndex int
			err = prompt.Select(ctx, &userIndex, "Select user:", "", userOptions...)
			if err != nil {
				return err
			}

			username = users[userIndex].Name
		}
		// If no users found, username remains empty and will use default credentials
	}

	// Database selection priority: flag > prompt result (if interactive) > credentials.DBName
	var db string
	if database := flag.GetString(ctx, "database"); database != "" {
		db = database
	} else if io.IsInteractive() {
		// Prompt for database selection
		databases, err := listConnectDatabases(ctx, useLegacy, clusterID)
		if err != nil {
			return fmt.Errorf("failed to list databases: %w", err)
		}

		if len(databases) > 0 {
			var dbOptions []string
			for _, database := range databases {
				dbOptions = append(dbOptions, database.Name)
			}

			var dbIndex int
			err = prompt.Select(ctx, &dbIndex, "Select database:", "", dbOptions...)
			if err != nil {
				return err
			}

			db = databases[dbIndex].Name
		}
	}

	cluster, params, credentials, err := connectParamsFromCluster(ctx, response, useLegacy, port, localProxyPort, username, resolvedOrgSlug)
	if err != nil {
		return err
	}

	maybeWarnNotReady(io.ErrOut, cluster)

	psqlPath, err := exec.LookPath("psql")
	if err != nil {
		fmt.Fprintf(io.Out, "Could not find psql in your $PATH. Install it or point your psql at: %s", "someurl")

		return err
	}

	// We want to handle cancels ourselves, since they can pass through
	// as query cancellations to psql without killing the proxy.
	proxyCtx, proxyCancel := context.WithCancel(context.WithoutCancel(ctx))
	defer proxyCancel()

	err = proxy.Start(proxyCtx, params)
	if err != nil {
		return err
	}

	connectUrl := buildConnectURL(credentials, db, localProxyPort)

	// Allow Ctrl+C signals to hit psql
	psqlCtx, psqlCancel := context.WithCancel(context.WithoutCancel(ctx))
	defer psqlCancel()

	cmd := exec.CommandContext(psqlCtx, psqlPath, connectUrl)
	cmd.Stdout = io.Out
	cmd.Stderr = io.ErrOut
	cmd.Stdin = io.In

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigChan)

	err = cmd.Start()
	if err != nil {
		return err
	}

	done := make(chan struct{})
	defer close(done)

	go func() {
		var lastSigTime time.Time

		for {
			select {
			case sig := <-sigChan:
				now := time.Now()

				if cmd.Process != nil {
					// Double Ctrl+C — kill the process
					if !lastSigTime.IsZero() && now.Sub(lastSigTime) < 2*time.Second {
						cmd.Process.Kill()
						psqlCancel()

						return
					}

					// Forward to psql for query cancellation
					cmd.Process.Signal(sig)
					lastSigTime = now
				}
			case <-done:
				return
			}
		}
	}()

	err = cmd.Wait()

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			// Check if the process was terminated by a signal (e.g., our Kill() call)
			if ws, ok := exitErr.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
				return nil
			}
		}
	}

	return err
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

// listConnectUsers lists users for the connect picker, using the same
// public/legacy source as getCluster resolved the cluster from. useLegacy
// is only true when the cluster could not be found through the public
// Machines API and was resolved via the legacy client instead.
func listConnectUsers(ctx context.Context, useLegacy bool, clusterID string) ([]mpgv2.User, error) {
	if useLegacy {
		mpgClient := mpgv2.ClientFromContext(ctx)

		usersResponse, err := mpgClient.ListUsers(ctx, clusterID)
		if err != nil {
			return nil, err
		}

		return usersResponse.Data, nil
	}

	return listUsers(ctx, clusterID)
}

// listConnectDatabases lists databases for the connect picker, using the
// same public/legacy source as getCluster resolved the cluster from.
func listConnectDatabases(ctx context.Context, useLegacy bool, clusterID string) ([]mpgv2.Database, error) {
	if useLegacy {
		mpgClient := mpgv2.ClientFromContext(ctx)

		databasesResponse, err := mpgClient.ListDatabases(ctx, clusterID)
		if err != nil {
			return nil, err
		}

		return databasesResponse.Data, nil
	}

	return listDatabases(ctx, flapsutil.ClientFromContext(ctx), clusterID)
}
