package machine

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/prompt"
	"github.com/superfly/flyctl/internal/render"
	"github.com/superfly/flyctl/iostreams"
)

func newEgressIp() *cobra.Command {
	const (
		long  = `Manage static egress (outgoing) IP addresses for machines`
		short = `Manage static egress IPs`
		usage = "egress-ip <command>"
	)

	cmd := command.New(usage, short, long, nil)

	cmd.Args = cobra.NoArgs

	cmd.AddCommand(
		newAllocateEgressIp(),
		newListEgressIps(),
	)

	return cmd
}

func newAllocateEgressIp() *cobra.Command {
	const (
		long  = `Allocate a pair of static egress IPv4 and IPv6 for a machine`
		short = `Allocate static egress IPs`
		usage = "allocate <machine-id>"
	)

	cmd := command.New("allocate", short, long, runAllocateEgressIP,
		command.RequireSession,
		command.LoadAppNameIfPresent,
	)

	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
	)

	cmd.Args = cobra.ExactArgs(1)

	return cmd
}

func newListEgressIps() *cobra.Command {
	const (
		long  = `List all allocated static egress IP addresses with their corresponding machine`
		short = `List all allocated static egress IPs`
	)

	cmd := command.New("list", short, long, runListEgressIps,
		command.RequireSession,
		command.LoadAppNameIfPresent,
	)

	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
	)

	cmd.Args = cobra.NoArgs

	return cmd
}

func runAllocateEgressIP(ctx context.Context) (err error) {
	var (
		args      = flag.Args(ctx)
		client    = flyutil.ClientFromContext(ctx)
		appName   = appconfig.NameFromContext(ctx)
		machineId = args[0]
	)

	if !flag.GetBool(ctx, "yes") {
		msg := `Looks like you're allocating a static egress (outgoing) IP. This is an advanced feature, and is not needed by most apps.
Are you sure this is what you want?`

		switch confirmed, err := prompt.Confirm(ctx, msg); {
		case err == nil:
			if !confirmed {
				return nil
			}
		case prompt.IsNonInteractive(err):
			return prompt.NonInteractiveError("yes flag must be specified when not running interactively")
		default:
			return err
		}
	}

	ipv4, ipv6, err := client.AllocateEgressIPAddress(ctx, appName, machineId)
	if err != nil {
		return err
	}

	fmt.Printf("Allocated egress IPs for machine %s:\n", machineId)
	fmt.Printf("IPv4: %s\n", ipv4.String())
	fmt.Printf("IPv6: %s\n", ipv6.String())
	return nil
}

func runListEgressIps(ctx context.Context) (err error) {
	var (
		client  = flyutil.ClientFromContext(ctx)
		appName = appconfig.NameFromContext(ctx)
	)

	machineIPs, err := client.GetEgressIPAddresses(ctx, appName)
	if err != nil {
		return err
	}

	rows := make([][]string, 0, 1)

	for machine, ips := range machineIPs {
		ipStr := make([]string, len(ips))
		for _, ip := range ips {
			ipStr = append(ipStr, ip.String())
		}
		rows = append(rows, []string{machine, strings.Join(ipStr, ",")})
	}

	out := iostreams.FromContext(ctx).Out
	render.Table(out, "", rows, "Machine", "Egress IPs")
	return nil
}
