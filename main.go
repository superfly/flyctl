package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"

	"github.com/logrusorgru/aurora"

	"github.com/superfly/flyctl/cmd"
	"github.com/superfly/flyctl/flyctl"
	"github.com/superfly/flyctl/internal/buildinfo"
	"github.com/superfly/flyctl/internal/client"
	"github.com/superfly/flyctl/internal/cmdutil"
	"github.com/superfly/flyctl/internal/env"
	"github.com/superfly/flyctl/internal/flyerr"
	"github.com/superfly/flyctl/internal/sentry"
	"github.com/superfly/flyctl/internal/update"
	"github.com/superfly/flyctl/terminal"
)

func main() {
	defer func() {
		if sentry.Recover() {
			os.Exit(1)
		}
	}()

	flyctl.InitConfig()

	if err := run(); err != nil {
		flyerr.PrintCLIOutput(err)

		flyctl.BackgroundTaskWG.Wait()

		os.Exit(1)
	}
}

func run() error {
	client := client.NewClient()
	if !client.IO.ColorEnabled() {
		// TODO: disable colors
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Kill, os.Interrupt)
	defer cancel()

	promptToUpdateIfRequired(ctx)

	root := cmd.NewRootCmd(client)
	_, err := root.ExecuteContextC(ctx)
	return err
}

func promptToUpdateIfRequired(ctx context.Context) {
	if !shouldCheckForUpdate() {
		return
	}

	terminal.Debug("Checking for updates...")

	currentVersion := buildinfo.Version()
	stateFilePath := filepath.Join(flyctl.ConfigDir(), "state.yml")

	newVersion, err := update.CheckForUpdate(ctx, stateFilePath, currentVersion)
	if err != nil {
		terminal.Debugf("error checking for update: %v", err)

		return
	}

	msg := fmt.Sprintf("Update available %s -> %s.\nRun \"%s\" to upgrade.",
		currentVersion,
		newVersion.Version,
		aurora.Bold(buildinfo.Name()+" version update"),
	)
	fmt.Fprintln(os.Stderr, aurora.Yellow(msg))
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

	if !buildinfo.IsRelease() || env.IsCI() || !cmdutil.IsTerminal(os.Stdout) || !cmdutil.IsTerminal(os.Stderr) {
		return false
	}

	return true
}
