package cmdv1

import (
	"context"
	"fmt"
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

func RunConnect(ctx context.Context, clusterID string, orgSlug string, proxyPort string) (err error) {
	io := iostreams.FromContext(ctx)

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
	// We'll get credentials from getMpgProxyParams, but need to prompt for database first if needed
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

	cluster, params, credentials, err := getMpgProxyParams(ctx, clusterID, proxyPort, username, orgSlug)
	if err != nil {
		return err
	}

	if cluster.Status != "ready" {
		fmt.Fprintf(io.ErrOut, "%s Cluster is not in ready state, currently: %s\n", aurora.Yellow("WARN"), cluster.Status)
	}

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

	user := credentials.User
	password := credentials.Password

	// Use selected database or fall back to default from credentials
	if db == "" {
		db = credentials.DBName
	}

	connectUrl := fmt.Sprintf("postgresql://%s:%s@localhost:%s/%s", user, password, proxyPort, db)

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
