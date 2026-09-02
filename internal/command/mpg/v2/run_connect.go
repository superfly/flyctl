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
	"github.com/superfly/flyctl/internal/prompt"
	mpgv2 "github.com/superfly/flyctl/internal/uiex/mpg/v2"
	"github.com/superfly/flyctl/iostreams"
	"github.com/superfly/flyctl/proxy"
)

func RunConnect(ctx context.Context, clusterID string, resolvedOrgSlug string, proxyPort string) (err error) {
	io := iostreams.FromContext(ctx)

	localProxyPort := proxyPort

	// Username selection: flag > prompt (if interactive) > empty (use default credentials)
	username := flag.GetString(ctx, "username")
	if username == "" && io.IsInteractive() {
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
	} else if io.IsInteractive() {
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

	cluster, useLegacy, params, credentials, err := GetMpgConnectParams(ctx, localProxyPort, username, clusterID, resolvedOrgSlug)
	if err != nil {
		return err
	}

	// Note: cluster.Status is classified inside GetMpgConnectParams (via
	// connectStatusRefusal in run_proxy.go) BEFORE any credential
	// resolution on the public path. Non-ready public statuses return a
	// deliberate, status-specific refusal error from there rather than
	// arriving here. The previous "Cluster is not in ready state" warning
	// was removed because it would either be dead code (every non-ready
	// public status is now refused upfront) or it would double-warn
	// against the deliberate refusal message. The legacy path
	// (useLegacy == true) intentionally does NOT apply
	// connectStatusRefusal — its original pre-migration credentials.Status
	// / credentials.Password checks fire post-fetch in
	// resolveConnectCredentials, and a non-ready-but-not-refused legacy
	// cluster (e.g. response.Data.Status == "creating" with valid
	// credentials — there is an existing "creating cluster with ready
	// credentials proceeds" test case proving this happens) reaches this
	// point the same way it did before the public-only classifier was
	// introduced. The pre-migration legacy warning is therefore restored
	// below, gated on useLegacy, so a non-ready legacy cluster connects
	// with a stderr warning instead of completely silently. The public
	// path (useLegacy == false) is intentionally silent: it refuses
	// non-ready statuses upfront, so by construction no non-ready cluster
	// reaches this point on the public path and the warning would only
	// ever fire for a "ready" status (i.e. never).
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
func buildConnectURL(credentials *mpgv2.GetClusterCredentialsResponse, db string, localProxyPort string) string {
	if db == "" {
		db = credentials.DBName
	}

	return fmt.Sprintf("postgresql://%s:%s@localhost:%s/%s", credentials.User, credentials.Password, localProxyPort, db)
}

// maybeWarnLegacyNotReady restores the ORIGINAL pre-migration legacy-path
// stderr warning that was removed when connectStatusRefusal was introduced:
// when the cluster lookup fell back to the legacy ui-ex client and that
// legacy response carries a non-ready cluster status, write a single
// yellow "WARN Cluster is not in ready state, currently: <status>" line to
// errOut and continue. On the public path the status classifier refuses
// non-ready statuses outright, so useLegacy == false short-circuits
// here — the warning is dead code on the public path and is gated at
// the call site so the regression surface (a non-ready cluster
// connecting completely silently on the legacy path) is explicitly
// closed.
//
// The warning format is preserved verbatim from the pre-migration code
// (commit 81f75427b^): aurora.Yellow wraps the literal "WARN", the
// status is interpolated after "currently: ", and the line ends with
// "\n". The function is intentionally a thin side-effecting helper so
// it can be unit-tested with a bytes.Buffer without involving the
// agent/establish code path or RunConnect's exec/psql machinery.
func maybeWarnLegacyNotReady(errOut io.Writer, useLegacy bool, cluster *mpgv2.ManagedCluster) {
	if !useLegacy || cluster == nil || cluster.Status == "ready" {
		return
	}

	fmt.Fprintf(errOut, "%s Cluster is not in ready state, currently: %s\n", aurora.Yellow("WARN"), cluster.Status)
}
