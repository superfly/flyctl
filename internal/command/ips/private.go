package ips

import (
	"context"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flag/flagnames"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/render"
	"github.com/superfly/flyctl/iostreams"
)

func newPrivate() *cobra.Command {
	const (
		long = `List the ID, region and private IP address of each of the app's
Machines, for connections over Fly's private network. With ` + "`--json`" + `,
print just the list of addresses. For the app's public and Flycast
addresses, use ` + "`fly ips list`" + `.`
		short = `List the private IP addresses of an app's Machines`
	)

	cmd := command.New("private", short, long, runPrivateIPAddressesList,
		command.RequireSession,
		command.RequireAppName,
	)

	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
		flag.JSONOutput(),
	)

	return cmd
}

func runPrivateIPAddressesList(ctx context.Context) error {
	appName := appconfig.NameFromContext(ctx)

	flapsClient := flapsutil.ClientFromContext(ctx)

	machines, _, err := flapsClient.ListFlyAppsMachines(ctx, appName)
	if err != nil {
		return err
	}

	if flag.GetBool(ctx, flagnames.JSONOutput) {
		privateIpAddresses := make([]string, 0, len(machines))

		for _, machine := range machines {
			if machine.PrivateIP != "" {
				privateIpAddresses = append(privateIpAddresses, machine.PrivateIP)
			}
		}

		out := iostreams.FromContext(ctx).Out

		return render.JSON(out, privateIpAddresses)
	} else {
		renderPrivateTableMachines(ctx, machines)
	}

	return nil
}
