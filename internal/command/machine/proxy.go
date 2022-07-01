package machine

import (
	"context"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/agent"
	"github.com/superfly/flyctl/internal/client"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/prompt"
	"github.com/superfly/flyctl/proxy"
)

func newProxy() *cobra.Command {
	const (
		short = "Establish a proxy to the Machine API through a Wireguard tunnel for local connections"
		long  = short + "\n"

		usage = "api-proxy"
	)

	cmd := command.New(usage, short, long, runMachineProxy, command.RequireSession)

	flag.Add(cmd, flag.Org())

	return cmd
}

func runMachineProxy(ctx context.Context) error {
	apiClient := client.FromContext(ctx).API()
	orgSlug := flag.GetOrg(ctx)

	if orgSlug == "" {
		org, err := prompt.Org(ctx)
		if err != nil {
			return err
		}
		orgSlug = org.Slug
	}

	if orgSlug != "" {
		_, err := apiClient.GetOrganizationBySlug(ctx, orgSlug)
		if err != nil {
			return err
		}
	}

	agentclient, err := agent.Establish(ctx, apiClient)
	if err != nil {
		return err
	}

	// do this explicitly so we can get the DNS server address
	_, err = agentclient.Establish(ctx, orgSlug)
	if err != nil {
		return err
	}

	dialer, err := agentclient.ConnectToTunnel(ctx, orgSlug)
	if err != nil {
		return err
	}

	// ports := strings.Split(args[0], ":")

	params := &proxy.ConnectParams{
		Ports:            []string{"4280"},
		OrganizationSlug: orgSlug,
		Dialer:           dialer,
		RemoteHost:       "_api.internal",
	}

	return proxy.Connect(ctx, params)
}
