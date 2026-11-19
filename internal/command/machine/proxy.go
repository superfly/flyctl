package machine

import (
	"context"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/agent"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/prompt"
	"github.com/superfly/flyctl/internal/uiexutil"
	"github.com/superfly/flyctl/proxy"
)

func newProxy() *cobra.Command {
	const (
		short = "Proxy the Machines API to a local port over WireGuard"
		long  = `Forward local port 4280 to the Machines API (_api.internal:4280) through a
WireGuard tunnel into an organization's private network, so local tools
can call the API. The proxy runs until you stop it.

Without ` + "`--org`" + `, you pick an organization in a terminal; if you belong
only to your personal organization, it's used automatically.`

		usage = "api-proxy"
	)

	cmd := command.New(usage, short, long, runMachineProxy, command.RequireSession)

	flag.Add(cmd,
		flag.Org(),
		flag.Bool{
			Name:        "quiet",
			Shorthand:   "q",
			Description: "Don't print progress indicators for WireGuard",
		},
	)

	return cmd
}

func runMachineProxy(ctx context.Context) error {
	apiClient := flyutil.ClientFromContext(ctx)
	orgSlug := flag.GetOrg(ctx)

	if orgSlug == "" {
		org, err := prompt.Org(ctx)
		if err != nil {
			return err
		}
		orgSlug = org.Slug
	}

	if orgSlug != "" {
		if _, err := uiexutil.ClientFromContext(ctx).GetOrganization(ctx, orgSlug); err != nil {
			return err
		}
	}

	agentclient, err := agent.Establish(ctx, apiClient)
	if err != nil {
		return err
	}

	// do this explicitly so we can get the DNS server address
	_, err = agentclient.Establish(ctx, orgSlug, "")
	if err != nil {
		return err
	}

	dialer, err := agentclient.ConnectToTunnel(ctx, orgSlug, "", flag.GetBool(ctx, "quiet"))
	if err != nil {
		return err
	}

	// ports := strings.Split(args[0], ":")

	params := &proxy.ConnectParams{
		BindAddr:         flag.GetBindAddr(ctx),
		Ports:            []string{"4280"},
		OrganizationSlug: orgSlug,
		Dialer:           dialer,
		RemoteHost:       "_api.internal",
	}

	return proxy.Connect(ctx, params)
}
