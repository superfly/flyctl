package redis

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/agent"
	"github.com/superfly/flyctl/gql"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/prompt"
	"github.com/superfly/flyctl/proxy"
	"github.com/superfly/flyctl/terminal"
)

func newProxy() (cmd *cobra.Command) {
	const (
		long = `Proxy to a Redis database`

		short = long
		usage = "proxy"
	)

	cmd = command.New(usage, short, long, runProxy, command.RequireSession)

	flag.Add(cmd,
		flag.Org(),
		flag.Region(),
	)

	return cmd
}

func runProxy(ctx context.Context) (err error) {
	localProxyPort := "16379"
	params, password, err := getRedisProxyParams(ctx, localProxyPort)
	if err != nil {
		return err
	}

	terminal.Infof("Proxying redis to port \"%s\" with password \"%s\"", localProxyPort, password)

	return proxy.Connect(ctx, params)
}

func getRedisProxyParams(ctx context.Context, localProxyPort string) (*proxy.ConnectParams, string, error) {
	client := flyutil.ClientFromContext(ctx)

	var index int
	var options []string

	result, err := gql.ListAddOns(ctx, client.GenqClient(), "upstash_redis")
	if err != nil {
		return nil, "", err
	}

	databases := result.AddOns.Nodes

	for _, database := range databases {
		options = append(options, fmt.Sprintf("%s (%s) %s", database.Name, database.PrimaryRegion, database.Organization.Slug))
	}

	err = prompt.Select(ctx, &index, "Select a database to connect to", "", options...)
	if err != nil {
		return nil, "", err
	}

	response, err := gql.GetAddOn(ctx, client.GenqClient(), databases[index].Name, string(gql.AddOnTypeUpstashRedis))
	if err != nil {
		return nil, "", err
	}

	database := response.AddOn

	agentclient, err := agent.Establish(ctx, client)
	if err != nil {
		return nil, "", err
	}

	dialer, err := agentclient.ConnectToTunnel(ctx, database.Organization.Slug, "", false)
	if err != nil {
		return nil, "", err
	}

	return &proxy.ConnectParams{
		Ports:            []string{localProxyPort, "6379"},
		OrganizationSlug: database.Organization.Slug,
		Dialer:           dialer,
		RemoteHost:       database.PrivateIp,
	}, database.Password, nil
}
