package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/hashicorp/go-multierror"
	"github.com/logrusorgru/aurora"
	"github.com/superfly/flyctl/cmd"
	"github.com/superfly/flyctl/flyctl"
	"github.com/superfly/flyctl/internal/client"
	"github.com/superfly/flyctl/internal/cmdutil"
	"github.com/superfly/flyctl/internal/update"
	"github.com/superfly/flyctl/terminal"
)

func main() {
	opts := sentry.ClientOptions{
		Dsn: "https://89fa584dc19b47a6952dd94bf72dbab4@sentry.io/4492967",
		// Debug:       true,
		Environment: flyctl.Environment,
		Release:     flyctl.Version,
		Transport: &sentry.HTTPSyncTransport{
			Timeout: 3 * time.Second,
		},
		BeforeSend: func(event *sentry.Event, hint *sentry.EventHint) *sentry.Event {
			if flyctl.Environment != "production" {
				return nil
			}
			return event
		},
	}
	if err := sentry.Init(opts); err != nil {
		fmt.Printf("sentry.Init: %s", err)
	}

	defer func() {
		if err := recover(); err != nil {
			sentry.CurrentHub().Recover(err)

			fmt.Println(aurora.Red("Oops, something went wrong! Could you try that again?"))

			if flyctl.Environment != "production" {
				fmt.Println()
				fmt.Println(err)
				fmt.Println(string(debug.Stack()))
			}

			os.Exit(1)
		}
	}()

	flyctl.InitConfig()

	updateChan := make(chan *update.Release)
	go func() {
		defer update.PostUpgradeCleanup()

		rel, err := checkForUpdate(flyctl.Version)
		if err != nil {
			terminal.Debug("error checking for update:", err)
		}
		updateChan <- rel
	}()

	client := client.NewClient()

	if !client.IO.ColorEnabled() {
		// disable colors
	}

	root := cmd.NewRootCmd(client)

	// cmd, _, err := root.Traverse(os.Args[1:])
	// fmt.Println("resolved to", cmd.Use)
	// checkErr(err)

	update := <-updateChan
	if update != nil {
		fmt.Fprintln(os.Stderr, aurora.Yellow(fmt.Sprintf("Update available %s -> %s", flyctl.Version, update.Version)))
		fmt.Fprintln(os.Stderr, aurora.Yellow(fmt.Sprintf("Run \"%s\" to upgrade", aurora.Bold(flyname.Name()+" version update"))))
	}

	_, err := root.ExecuteC()
	checkErr(err)
}

func checkErr(err error) {
	if err == nil {
		return
	}

	if !isCancelledError(err) {
		fmt.Println(aurora.Red("Error"), err)
	}

	safeExit()
}

func isCancelledError(err error) bool {
	if err == cmd.ErrAbort {
		return true
	}

	if err == context.Canceled {
		return true
	}

	if merr, ok := err.(*multierror.Error); ok {
		if len(merr.Errors) == 1 && merr.Errors[0] == context.Canceled {
			return true
		}
	}

	return false
}

func safeExit() {
	flyctl.BackgroundTaskWG.Wait()

	os.Exit(1)
}

func checkForUpdate(currentVersion string) (*update.Release, error) {
	if !shouldCheckForUpdate() {
		return nil, nil
	}

	stateFilePath := filepath.Join(flyctl.ConfigDir(), "state.yml")
	return update.CheckForUpdate(context.Background(), stateFilePath, currentVersion)
}

func shouldCheckForUpdate() bool {
	// for testing
	if os.Getenv("FLY_UPDATE_CHECK") == "1" {
		return true
	}

	if os.Getenv("FLY_NO_UPDATE_CHECK") != "" {
		return false
	}
	if os.Getenv("CODESPACES") != "" {
		return false
	}

	if flyctl.Environment != "production" || isCI() || !cmdutil.IsTerminal(os.Stdout) || !cmdutil.IsTerminal(os.Stderr) {
		return false
	}

	return true
}

// based on https://github.com/watson/ci-info/blob/HEAD/index.js
func isCI() bool {
	return os.Getenv("CI") != "" || // GitHub Actions, Travis CI, CircleCI, Cirrus CI, GitLab CI, AppVeyor, CodeShip, dsari
		os.Getenv("BUILD_NUMBER") != "" || // Jenkins, TeamCity
		os.Getenv("RUN_ID") != "" // TaskCluster, dsari
}
