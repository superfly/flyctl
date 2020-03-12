package main

import (
	"fmt"
	"runtime/debug"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/logrusorgru/aurora"
	"github.com/superfly/flyctl/cmd"
	"github.com/superfly/flyctl/flyctl"
)

func main() {
	opts := sentry.ClientOptions{
		Dsn: "https://89fa584dc19b47a6952dd94bf72dbab4@sentry.io/4492967",
		// Debug:       true,
		Environment: flyctl.Environment,
		Release:     flyctl.Version,
	}
	if err := sentry.Init(opts); err != nil {
		fmt.Printf("sentry.Init: %s", err)
	}
	defer sentry.Flush(2 * time.Second)

	defer func() {
		if err := recover(); err != nil {
			sentry.CurrentHub().Recover(err)

			fmt.Println(aurora.Red("Oops, something went wrong! Could you try that again?"))

			if flyctl.Environment != "production" {
				fmt.Println()
				fmt.Println(err)
				fmt.Println(string(debug.Stack()))
			}
		}
	}()

	cmd.Execute()
}
