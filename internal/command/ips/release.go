package ips

import (
	"context"
	"fmt"
	"net"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flapsutil"
)

func newRelease() *cobra.Command {
	const (
		long = `Release one or more public or private IP addresses from the app,
immediately and without a confirmation prompt. Pass the addresses as
arguments; find them with ` + "`fly ips list`" + `.`
		short = `Release ingress IP addresses`
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

func newReleaseEgress() *cobra.Command {
	const (
		long = `Release one or more egress IP addresses from the app, immediately and
without a confirmation prompt. Pass the addresses as arguments; find them
with ` + "`fly ips list`" + `.`
		short = `Release egress IP addresses`
	)

	cmd := command.New("release-egress [flags] ADDRESS ADDRESS ...", short, long, runReleaseEgressIPAddress,
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
	return releaseIPAddresses(ctx, flag.Args(ctx))
}

func runReleaseEgressIPAddress(ctx context.Context) error {
	if err := releaseIPAddresses(ctx, flag.Args(ctx)); err != nil {
		return err
	}

	SanityCheckAppScopedEgressIps(ctx, nil, nil, nil, "")

	return nil
}

// releaseIPAddresses releases the given addresses from the app. The Machines API uses the same
// endpoint for ingress and egress IPs.
func releaseIPAddresses(ctx context.Context, addresses []string) error {
	flapsClient := flapsutil.ClientFromContext(ctx)
	appName := appconfig.NameFromContext(ctx)

	for _, address := range addresses {
		if ip := net.ParseIP(address); ip == nil {
			return fmt.Errorf("Invalid IP address: '%s'", address)
		}

		if err := flapsClient.DeleteIPAssignment(ctx, appName, address); err != nil {
			return err
		}

		fmt.Printf("Released %s from %s\n", address, appName)
	}

	return nil
}
