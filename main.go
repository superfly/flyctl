package main

import (
	"context"
	"fmt"
	"os"
	"runtime/debug"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/hashicorp/go-multierror"
	"github.com/logrusorgru/aurora"
	"github.com/superfly/flyctl/cmd"
	"github.com/superfly/flyctl/flyctl"
	"github.com/superfly/flyctl/flyname"
	"github.com/superfly/flyctl/internal/client"
)

func main() {
	flyname.Name() // Initialise
	opts := sentry.ClientOptions{
		Dsn: "https://89fa584dc19b47a6952dd94bf72dbab4@sentry.io/4492967",
		// Debug:       true,
		Environment: flyctl.Environment,
		Release:     flyctl.Version,
	}
	if err := sentry.Init(opts); err != nil {
		fmt.Printf("sentry.Init: %s", err)
	}

	defer func() {
		if err := recover(); err != nil {
			sentry.CurrentHub().Recover(err)

			fmt.Println(aurora.Red("Oops, something went wrong! Could you try that again?"))

			if flyctl.Environment == "production" {
				flyctl.BackgroundTaskWG.Add(1)
				go func() {
					sentry.Flush(2 * time.Second)
					flyctl.BackgroundTaskWG.Done()
				}()
			} else {
				if flyctl.Environment != "production" {
					fmt.Println()
					fmt.Println(err)
					fmt.Println(string(debug.Stack()))
				}
			}

			flyctl.BackgroundTaskWG.Wait()

			os.Exit(1)
		}
	}()

	defer flyctl.BackgroundTaskWG.Wait()

	client := client.NewClient()

	if !client.IO.ColorEnabled() {
		// disable colors
	}

	root := cmd.NewRootCmd(client)

	// cmd, _, err := root.Traverse(os.Args[1:])
	// fmt.Println("resolved to", cmd.Use)
	// checkErr(err)

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
