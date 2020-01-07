package cmd

import (
	"fmt"
	"github.com/superfly/flyctl/docstrings"
	"math"
	"os"
	"time"

	"github.com/logrusorgru/aurora"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/terminal"
)

func newAppLogsCommand() *Command {
	logsStrings := docstrings.Get("logs")
	cmd := BuildCommand(nil, runLogs, logsStrings.Usage, logsStrings.Short, logsStrings.Long, true, os.Stdout, requireAppName)

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

func runLogs(ctx *CmdContext) error {
	errorCount := 0
	emptyCount := 0
	instanceFilter, _ := ctx.Config.GetString("instance")
	regionFilter, _ := ctx.Config.GetString("region")

	nextToken := ""

	for {
		entries, token, err := ctx.FlyClient.GetAppLogs(ctx.AppName, nextToken, regionFilter, instanceFilter)

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

			printLogEntries(entries)

			if token != "" {
				nextToken = token
			}
		}
	}

	return nil
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

func printLogEntries(entries []api.LogEntry) {
	for _, entry := range entries {
		fmt.Printf(
			"%s %s %s [%s] %s\n",
			aurora.Faint(entry.Timestamp),
			entry.Meta.Instance,
			aurora.Green(entry.Meta.Region),
			aurora.Colorize(entry.Level, levelColor(entry.Level)),
			entry.Message,
		)
	}
}

func levelColor(level string) aurora.Color {
	switch level {
	case "debug":
		return aurora.CyanFg
	case "info":
		return aurora.BlueFg
	case "warning":
		return aurora.MagentaFg
	}
	return aurora.RedFg
}
