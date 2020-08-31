package cmd

import (
	"math"
	"os"
	"time"

	"github.com/superfly/flyctl/cmd/presenters"
	"github.com/superfly/flyctl/cmdctx"

	"github.com/superfly/flyctl/docstrings"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/terminal"
)

func newLogsCommand() *Command {
	logsStrings := docstrings.Get("logs")
	cmd := BuildCommandKS(nil, runLogs, logsStrings, os.Stdout, requireSession, requireAppName)

	// TODO: Move flag descriptions into the docStrings
	cmd.AddStringFlag(StringFlagOpts{
		Name:        "instance",
		Shorthand:   "i",
		Description: "Filter by instance ID",
	})
	cmd.AddStringFlag(StringFlagOpts{
		Name:        "region",
		Shorthand:   "r",
		Description: "Filter by region",
	})

	return cmd
}

func runLogs(ctx *cmdctx.CmdContext) error {
	errorCount := 0
	emptyCount := 0
	instanceFilter, _ := ctx.Config.GetString("instance")
	regionFilter, _ := ctx.Config.GetString("region")

	nextToken := ""

	logPresenter := presenters.LogPresenter{}

	for {
		entries, token, err := ctx.Client.API().GetAppLogs(ctx.AppName, nextToken, regionFilter, instanceFilter)

		if err != nil {
			if api.IsNotAuthenticatedError(err) {
				return err
			} else if api.IsNotFoundError(err) {
				return err
			} else {
				errorCount++
				if errorCount > 3 {
					return err
				}
				sleep(errorCount)
			}
		}
		errorCount = 0

		if len(entries) == 0 {
			emptyCount++
			sleep(emptyCount)
		} else {
			emptyCount = 0

			logPresenter.FPrint(ctx.Out, ctx.OutputJSON(), entries)

			if token != "" {
				nextToken = token
			}
		}
	}

	// This should not be reached
}

var maxBackoff float64 = 5000

func sleep(backoffCount int) {
	sleepTime := math.Pow(float64(backoffCount), 2) * 250
	if sleepTime > maxBackoff {
		sleepTime = maxBackoff
	}
	terminal.Debug("backoff ms:", sleepTime)
	time.Sleep(time.Duration(sleepTime) * time.Millisecond)
}
