package ips

import (
	"context"
	"fmt"
	"net"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flyutil"
)

func newRelease() *cobra.Command {
	const (
		long  = `Releases one or more IP addresses from the application`
		short = `Release IP addresses`
	)

	cmd := command.New("release [flags] ADDRESS ADDRESS ...", short, long, runReleaseIPAddress,
		command.RequireSession,
		command.RequireAppName,
	)

	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
	)

	cmd.Args = cobra.MinimumNArgs(1)
	return cmd
}

func runReleaseIPAddress(ctx context.Context) error {
	client := flyutil.ClientFromContext(ctx)

	appName := appconfig.NameFromContext(ctx)

	for _, address := range flag.Args(ctx) {

		if ip := net.ParseIP(address); ip == nil {
			return fmt.Errorf("Invalid IP address: '%s'", address)
		}

		if err := client.ReleaseIPAddress(ctx, appName, address); err != nil {
			return err
		}

		fmt.Printf("Released %s from %s\n", address, appName)
	}

	return nil
}
