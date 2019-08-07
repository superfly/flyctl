package cmd

import (
	"fmt"
	"math"
	"os"
	"time"

	"github.com/logrusorgru/aurora"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/terminal"
)

func newAppLogsCommand() *Command {
	return BuildCommand(runLogs, "logs", "view app logs", os.Stdout, true, requireAppName)
}

func runLogs(ctx *CmdContext) error {
	errorCount := 0
	emptyCount := 0

	nextToken := ""

	for {
		entries, token, err := api.GetAppLogs(ctx.AppName(), nextToken)

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
