package ips

import (
	"context"
	"fmt"
	"reflect"

	"github.com/spf13/cobra"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/machine"
	"github.com/superfly/flyctl/internal/prompt"
	"github.com/superfly/flyctl/iostreams"
)

func newAllocate() *cobra.Command {
	const (
		long  = `Allocate recommended IP addresses for the application`
		short = `Allocate recommended IP addresses`
	)

	cmd := command.New("allocate", short, long, runAllocateInteractive,
		command.RequireSession,
		command.RequireAppName,
	)

	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
		flag.Region(),
	)

	return cmd
}

func determineIPTypeFromDeployedServices(ctx context.Context, appName string) (requiresDedicated bool, hasServices bool, hasUDP bool, err error) {
	flapsClient, err := flapsutil.NewClientWithOptions(ctx, flaps.NewClientOpts{
		AppName: appName,
	})
	if err != nil {
		return false, false, false, fmt.Errorf("could not create flaps client: %w", err)
	}
	ctx = flapsutil.NewContextWithClient(ctx, flapsClient)

	machines, err := machine.ListActive(ctx)
	if err != nil {
		return false, false, false, fmt.Errorf("could not list machines: %w", err)
	}

	if len(machines) == 0 {
		return false, false, false, nil
	}

	hasServices = false
	hasUDP = false
	requiresDedicated = false

	for _, machine := range machines {
		if machine.Config == nil {
			continue
		}

		for _, service := range machine.Config.Services {
			hasServices = true

			switch service.Protocol {
			case "udp":
				hasUDP = true
			case "tcp":
				for _, port := range service.Ports {
					if port.HasNonHttpPorts() {
						requiresDedicated = true
					} else if port.ContainsPort(80) && !reflect.DeepEqual(port.Handlers, []string{"http"}) {
						requiresDedicated = true
					} else if port.ContainsPort(443) && !(reflect.DeepEqual(port.Handlers, []string{"http", "tls"}) || reflect.DeepEqual(port.Handlers, []string{"tls", "http"})) {
						requiresDedicated = true
					}
				}
			}
		}
	}

	return requiresDedicated, hasServices, hasUDP, nil
}

func runAllocateInteractive(ctx context.Context) error {
	client := flyutil.ClientFromContext(ctx)
	appName := appconfig.NameFromContext(ctx)
	io := iostreams.FromContext(ctx)
	colorize := io.ColorScheme()

	requiresDedicated, hasServices, hasUDP, err := determineIPTypeFromDeployedServices(ctx, appName)
	if err != nil {
		return fmt.Errorf("failed to check deployed services: %w", err)
	}

	if !hasServices {
		fmt.Fprintln(io.Out, "No services are currently deployed on this app.")
		fmt.Fprintln(io.Out, "IP addresses are only needed if you have services with external ports configured.")

		confirmed, err := prompt.Confirm(ctx, "Would you like to allocate IP addresses anyway?")
		if err != nil {
			if prompt.IsNonInteractive(err) {
				return prompt.NonInteractiveError("use fly ips allocate-v4 or fly ips allocate-v6 in non-interactive mode")
			}
			return err
		}
		if !confirmed {
			return nil
		}
	}

	existingIPs, err := client.GetIPAddresses(ctx, appName)
	if err != nil {
		return fmt.Errorf("failed to get existing IP addresses: %w", err)
	}

	hasV4 := false
	hasSharedV4 := false
	hasV6 := false
	for _, ip := range existingIPs {
		if ip.Type == "v4" {
			hasV4 = true
		}
		if ip.Type == "shared_v4" {
			hasSharedV4 = true
		}
		if ip.Type == "v6" {
			hasV6 = true
		}
	}

	if len(existingIPs) > 0 {
		fmt.Fprint(io.Out, "Your app already has the following IP addresses:\n\n")
		renderListTable(ctx, existingIPs)
	}

	recommendDedicated := requiresDedicated && hasSharedV4 && !hasV4
	if (hasV4 || hasSharedV4) && hasV6 && !recommendDedicated {
		fmt.Fprintln(io.Out, "Your app has all necessary IP addresses.")
		fmt.Fprintln(io.Out, "To allocate more addresses, run:")
		fmt.Fprintf(io.Out, "   %s (dedicated IPv4)\n", colorize.Bold("fly ips allocate-v4"))
		if !hasSharedV4 {
			fmt.Fprintf(io.Out, "   %s (shared IPv4)\n", colorize.Bold("fly ips allocate-v4 --shared"))
		}
		fmt.Fprintf(io.Out, "   %s (dedicated IPv6)\n", colorize.Bold("fly ips allocate-v6"))
		fmt.Fprintf(io.Out, "   %s (private IPv6)\n", colorize.Bold("fly ips allocate-v6 --private"))
		return nil
	}

	allocateV6 := false
	allocateSharedV4 := false
	allocateDedicatedV4 := false
	msg := ""

	if recommendDedicated {
		msg = `Your app has a service that requires a dedicated IPv4 address, but you currently have a shared IPv4.
Would you like to allocate a dedicated IPv4 address?
    IPv4: Dedicated ($2/mo)`

		allocateDedicatedV4 = true
	} else if hasUDP && !hasV4 {
		msg = `Your app has a UDP service that requires a dedicated IPv4 address.
Would you like to allocate the following addresses?
    IPv4: Dedicated ($2/mo)
    IPv6: None (Fly.io does not support UDP over public IPv6)`

		allocateDedicatedV4 = true
	} else if !hasV4 && !hasV6 && !hasSharedV4 {
		if requiresDedicated {
			msg = `Your app has a service that requires a dedicated IPv4 address.
Would you like to allocate the following addresses?
    IPv4: Dedicated ($2/mo)
    IPv6: Dedicated (no charge)`

			allocateDedicatedV4 = true
			allocateV6 = true
		} else {
			msg = `Would you like to allocate the following addresses?
    IPv4: Shared (no charge)
    IPv6: Dedicated (no charge)`

			allocateSharedV4 = true
			allocateV6 = true
		}
	} else if !hasV4 && !hasSharedV4 {
		if requiresDedicated {
			msg = `Your app has a service that requires a dedicated IPv4 address.
Would you like to allocate the following address?
    IPv4: Dedicated ($2/mo)`

			allocateDedicatedV4 = true
		} else {
			msg = `Would you like to allocate the following address?
    IPv4: Shared (no charge)`

			allocateSharedV4 = true
		}
	} else if !hasV6 {
		msg = `Would you like to allocate the following address?
    IPv6: Dedicated (no charge)`

		allocateV6 = true
	}

	if len(msg) == 0 {
		return nil
	}

	confirmed, err := prompt.Confirm(ctx, msg)
	if err != nil {
		if prompt.IsNonInteractive(err) {
			return prompt.NonInteractiveError("use fly ips allocate-v4 or fly ips allocate-v6 in non-interactive mode")
		}
		return err
	}

	if !confirmed {
		fmt.Fprintln(io.Out, "\nTo customize your IP allocations, run:")
		fmt.Fprintf(io.Out, "   %s (dedicated IPv4)\n", colorize.Bold("fly ips allocate-v4"))
		if !hasSharedV4 {
			fmt.Fprintf(io.Out, "   %s (shared IPv4)\n", colorize.Bold("fly ips allocate-v4 --shared"))
		}
		fmt.Fprintf(io.Out, "   %s (dedicated IPv6)\n", colorize.Bold("fly ips allocate-v6"))
		fmt.Fprintf(io.Out, "   %s (private IPv6)\n", colorize.Bold("fly ips allocate-v6 --private"))
		return nil
	}
	fmt.Fprintln(io.Out, "")

	if allocateSharedV4 {
		fmt.Fprintln(io.Out, "Allocating shared IPv4...")
		ipAddress, err := client.AllocateSharedIPAddress(ctx, appName)
		if err != nil {
			return err
		}

		renderSharedTable(ctx, ipAddress)
	}

	if allocateDedicatedV4 {
		fmt.Fprintln(io.Out, "Allocating dedicated IPv4...")
		region := flag.GetRegion(ctx)
		ipAddress, err := client.AllocateIPAddress(ctx, appName, "v4", region, nil, "")
		if err != nil {
			return fmt.Errorf("failed to allocate dedicated IPv4: %w", err)
		}

		ipAddresses := []fly.IPAddress{*ipAddress}
		renderListTable(ctx, ipAddresses)
	}

	if allocateV6 {
		fmt.Fprintln(io.Out, "Allocating IPv6...")
		region := flag.GetRegion(ctx)
		ipAddress, err := client.AllocateIPAddress(ctx, appName, "v6", region, nil, "")
		if err != nil {
			return fmt.Errorf("failed to allocate IPv6: %w", err)
		}

		ipAddresses := []fly.IPAddress{*ipAddress}
		renderListTable(ctx, ipAddresses)
	}

	if allocateSharedV4 && !hasV4 {
		fmt.Fprintf(io.Out, "Note: You've been allocated a shared IPv4 address. To get a dedicated IPv4 address, run: %s\n", colorize.Bold("fly ips allocate-v4"))
	}

	return nil
}
