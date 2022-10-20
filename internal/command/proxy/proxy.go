package proxy

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/agent"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/internal/app"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/prompt"
	"github.com/superfly/flyctl/proxy"
)

func New() *cobra.Command {
	var (
		long  = strings.Trim(`Proxies connections to a fly VM through a Wireguard tunnel The current application DNS is the default remote host`, "\n")
		short = `Proxies connections to a fly VM`
	)

	cmd := command.New("proxy <local:remote> [remote_host]", short, long, run,
		command.RequireSession, command.LoadAppNameIfPresent)

	cmd.Args = cobra.RangeArgs(1, 2)

	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
		flag.Org(),
		flag.Bool{
			Name:        "select",
			Shorthand:   "s",
			Default:     false,
			Description: "Prompt to select from available instances from the current application",
		},
		flag.Bool{
			Name:        "quiet",
			Shorthand:   "q",
			Description: "Don't print progress indicators for WireGuard",
		},
	)

	return cmd
}

func run(ctx context.Context) (err error) {
	client := client.FromContext(ctx).API()
	appName := app.NameFromContext(ctx)
	orgSlug := flag.GetString(ctx, "org")
	args := flag.Args(ctx)
	promptInstance := flag.GetBool(ctx, "select")

	if promptInstance && appName == "" {
		return errors.New("--app required when --select flag provided")
	}

	if orgSlug != "" {
		_, err := client.GetOrganizationBySlug(ctx, orgSlug)
		if err != nil {
			return err
		}
	}

	if appName == "" && orgSlug == "" {
		org, err := prompt.Org(ctx)
		if err != nil {
			return err
		}
		orgSlug = org.Slug
	}

	// var app *api.App
	if appName != "" {
		app, err := client.GetAppBasic(ctx, appName)
		if err != nil {
			return err
		}
		orgSlug = app.Organization.Slug
	}

	agentclient, err := agent.Establish(ctx, client)
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

	ports := strings.Split(args[0], ":")

	params := &proxy.ConnectParams{
		Ports:            ports,
		AppName:          appName,
		OrganizationSlug: orgSlug,
		Dialer:           dialer,
		PromptInstance:   promptInstance,
	}

	// FIXME: lookup remote host with gql+dns, prefer gql if diff
	if len(args) > 1 {
		params.RemoteHost = args[1]
	} else {
		params.RemoteHost = fmt.Sprintf("%s.internal", appName)
	}

	return proxy.Connect(ctx, params)
}
