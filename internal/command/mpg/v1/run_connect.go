package cmdv1

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
	"github.com/superfly/flyctl/internal/prompt"
	mpgv1 "github.com/superfly/flyctl/internal/uiex/mpg/v1"
	"github.com/superfly/flyctl/iostreams"
	"github.com/superfly/flyctl/proxy"
)

func RunConnect(ctx context.Context, clusterID string, resolvedOrgSlug string) (err error) {
	io := iostreams.FromContext(ctx)

	localProxyPort := "16380"

	// Username selection: flag > prompt (if interactive) > empty (use default credentials)
	username := flag.GetString(ctx, "username")
	if username == "" && io.IsInteractive() {
		// Prompt for user selection
		mpgClient := mpgv1.ClientFromContext(ctx)
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
	} else if io.IsInteractive() {
		// Prompt for database selection
		mpgClient := mpgv1.ClientFromContext(ctx)
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

	cluster, useLegacy, params, credentials, err := GetMpgConnectParams(ctx, localProxyPort, username, clusterID, resolvedOrgSlug)
	if err != nil {
		return err
	}

	// Gated on useLegacy; see maybeWarnLegacyNotReady's doc comment for why.
	maybeWarnLegacyNotReady(io.ErrOut, useLegacy, cluster)

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

// buildConnectURL composes the psql connection URL from resolved credentials
// and the proxy port. db follows the priority used by RunConnect: an explicit
// --database value (or interactive prompt result) wins over credentials.DBName.
// credentials.DBName is the plan-required default ("fly-db") on both the
// public default-user and public explicit-user paths, so a non-interactive
// `fly mpg connect <cluster> --user alice` lands on postgresql://.../fly-db.
func buildConnectURL(credentials *mpgv1.GetManagedClusterCredentialsResponse, db string, localProxyPort string) string {
	if db == "" {
		db = credentials.DBName
	}

	return fmt.Sprintf("postgresql://%s:%s@localhost:%s/%s", credentials.User, credentials.Password, localProxyPort, db)
}

// maybeWarnLegacyNotReady restores the pre-migration legacy-path "Cluster
// is not in ready state" stderr warning. It is gated on useLegacy so the
// public path (which already refuses non-ready clusters via
// connectStatusRefusal) stays silent — see connectStatusRefusal's doc
// comment for the legacy/public status-split rationale. The warning
// format is preserved verbatim from the pre-migration code
// (commit 81f75427b^): aurora.Yellow("WARN") + " Cluster is not in ready
// state, currently: <status>\n". The function is a thin side-effecting
// helper so it can be unit-tested with a bytes.Buffer without involving
// the agent/establish code path or RunConnect's exec/psql machinery.
func maybeWarnLegacyNotReady(errOut io.Writer, useLegacy bool, cluster *mpgv1.ManagedCluster) {
	if !useLegacy || cluster == nil || cluster.Status == "ready" {
		return
	}

	fmt.Fprintf(errOut, "%s Cluster is not in ready state, currently: %s\n", aurora.Yellow("WARN"), cluster.Status)
}
