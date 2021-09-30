package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"time"

	"github.com/blang/semver"
	"github.com/getsentry/sentry-go"
	"github.com/logrusorgru/aurora"

	"github.com/superfly/flyctl/cmd"
	"github.com/superfly/flyctl/flyctl"
	"github.com/superfly/flyctl/internal/buildinfo"
	"github.com/superfly/flyctl/internal/client"
	"github.com/superfly/flyctl/internal/cmdutil"
	"github.com/superfly/flyctl/internal/flyerr"
	"github.com/superfly/flyctl/internal/update"
	"github.com/superfly/flyctl/terminal"
)

func main() {
	opts := sentry.ClientOptions{
		Dsn: "https://89fa584dc19b47a6952dd94bf72dbab4@sentry.io/4492967",
		// Debug:       true,
		Environment: buildinfo.Environment(),
		Release:     buildinfo.Version().String(),
		Transport: &sentry.HTTPSyncTransport{
			Timeout: 3 * time.Second,
		},
		BeforeSend: func(event *sentry.Event, hint *sentry.EventHint) *sentry.Event {
			if buildinfo.IsDev() {
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

			var buf bytes.Buffer

			fmt.Fprintln(&buf, aurora.Red("Oops, something went wrong! Could you try that again?"))

			if buildinfo.IsDev() {
				fmt.Fprintln(&buf)
				fmt.Fprintln(&buf, err)
				fmt.Fprintln(&buf, string(debug.Stack()))
			}

			buf.WriteTo(os.Stdout)

			os.Exit(1)
		}
	}()

	flyctl.InitConfig()

	updateChan := make(chan *update.Release)
	go func() {
		defer update.PostUpgradeCleanup()

		rel, err := checkForUpdate(buildinfo.Version())
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
		fmt.Fprintln(os.Stderr, aurora.Yellow(fmt.Sprintf("Update available %s -> %s", buildinfo.Version(), update.Version)))
		fmt.Fprintln(os.Stderr, aurora.Yellow(fmt.Sprintf("Run \"%s\" to upgrade", aurora.Bold(buildinfo.Name()+" version update"))))
	}

	_, err := root.ExecuteC()
	checkErr(err)
}

func checkErr(err error) {
	if err == nil {
		return
	}

	flyerr.PrintCLIOutput(err)

	// if !isCancelledError(err) {
	// 	fmt.Println(aurora.Red("Error"), err)
	// }

	// if msg := flyerr.GetErrorDescription(err); msg != "" {

	// 	fmt.Printf("\n%s\n", msg)
	// }

	// if msg := flyerr.GetErrorSuggestion(err); msg != "" {
	// 	fmt.Printf("\n%s\n", msg)
	// }

	safeExit()
}

// func isCancelledError(err error) bool {
// 	if errors.Is(err, cmd.ErrAbort) {
// 		return true
// 	}

// 	if errors.Is(err, context.Canceled) {
// 		return true
// 	}

// 	// if err == cmd.ErrAbort {
// 	// 	return true
// 	// }

// 	// if err == context.Canceled {
// 	// 	return true
// 	// }

// 	// if merr, ok := err.(*multierror.Error); ok {
// 	// 	if len(merr.Errors) == 1 && merr.Errors[0] == context.Canceled {
// 	// 		return true
// 	// 	}
// 	// }

// 	return false
// }

func safeExit() {
	flyctl.BackgroundTaskWG.Wait()

	os.Exit(1)
}

func checkForUpdate(currentVersion semver.Version) (*update.Release, error) {
	if !shouldCheckForUpdate() {
		return nil, nil
	}
	terminal.Debug("Checking for updates...")

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

	if !buildinfo.IsRelease() || isCI() || !cmdutil.IsTerminal(os.Stdout) || !cmdutil.IsTerminal(os.Stderr) {
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
