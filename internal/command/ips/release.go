package ips

import (
	"context"
	"fmt"
	"net"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/internal/app"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
)

func newRelease() *cobra.Command {
	const (
		long  = `Releases an IP address from the application`
		short = `Release an IP address`
	)

	cmd := command.New("release [ADDRESS]", short, long, runReleaseIPAddress,
		command.RequireSession,
		command.RequireAppName,
	)

	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
	)

	cmd.Args = cobra.ExactArgs(1)
	return cmd
}

func runReleaseIPAddress(ctx context.Context) error {
	client := client.FromContext(ctx).API()

	appName := app.NameFromContext(ctx)
	address := flag.Args(ctx)[0]

	if ip := net.ParseIP(address); ip == nil {
		return fmt.Errorf("Invalid IP address: '%s'", address)
	}

	ipAddress, err := client.FindIPAddress(ctx, appName, address)
	if err != nil {
		return err
	}

	if err := client.ReleaseIPAddress(ctx, ipAddress.ID); err != nil {
		return err
	}

	fmt.Printf("Released %s from %s\n", ipAddress.Address, appName)

	return nil
}
