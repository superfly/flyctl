package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"syscall"

	dockerclient "github.com/docker/docker/client"
	dockeropts "github.com/docker/docker/opts"

	"github.com/superfly/flyctl/pkg/iostreams"

	"github.com/superfly/flyctl/internal/buildinfo"
	"github.com/superfly/flyctl/internal/cli"
	"github.com/superfly/flyctl/internal/sentry"
)

func main() {
	guessDockerHost()

	os.Exit(run())
}

func run() (exitCode int) {
	defer sentry.Flush()

	ctx, cancel := newContext()
	defer cancel()

	if !buildinfo.IsDev() {
		defer func() {
			if r := recover(); r != nil {
				sentry.Recover(r)

				exitCode = 3
			}
		}()
	}

	exitCode = cli.Run(ctx, iostreams.System(), os.Args[1:]...)

	return
}

func newContext() (context.Context, context.CancelFunc) {
	// NOTE: when signal.Notify is called for os.Interrupt it traps both
	// ^C (Control-C) and ^BREAK (Control-Break) on Windows.

	signals := []os.Signal{os.Interrupt}
	if runtime.GOOS != "windows" {
		signals = append(signals, syscall.SIGTERM)
	}

	return signal.NotifyContext(context.Background(), signals...)
}

func guessDockerHost() {
	host := os.Getenv("DOCKER_HOST")
	if host == "" {
		return // no docker host specified
	}

	if _, err := dockerclient.ParseHostURL(host); err == nil {
		return // host is well defined
	}

	host, err := dockeropts.ParseHost(false, false, host)
	if err != nil {
		return
	}

	if err := os.Setenv("DOCKER_HOST", host); err != nil {
		panic(fmt.Errorf("failed fixing DOCKER_HOST: %w", err))
	}
}
