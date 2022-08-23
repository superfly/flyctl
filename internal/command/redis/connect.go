package redis

import (
	"context"
	"fmt"
	"os/exec"
	"time"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/agent"
	"github.com/superfly/flyctl/gql"
	"github.com/superfly/flyctl/iostreams"
	"github.com/superfly/flyctl/proxy"

	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/prompt"
)

func newConnect() (cmd *cobra.Command) {
	const (
		long = `Connect to a Redis database using redis-cli`

		short = long
		usage = "connect"
	)

	cmd = command.New(usage, short, long, runConnect, command.RequireSession)

	flag.Add(cmd,
		flag.Org(),
		flag.Region(),
	)

	return cmd
}

func runConnect(ctx context.Context) (err error) {
	var (
		client = client.FromContext(ctx).API()
		io     = iostreams.FromContext(ctx)
	)

	if err != nil {
		return err
	}

	var index int
	var options []string

	result, err := gql.ListAddOns(ctx, client.GenqClient, "redis")
	if err != nil {
		return
	}

	databases := result.AddOns.Nodes

	for _, database := range databases {
		options = append(options, fmt.Sprintf("%s (%s) %s", database.Name, database.PrimaryRegion, database.Organization.Slug))
	}

	err = prompt.Select(ctx, &index, "Select a database to connect to", "", options...)
	if err != nil {
		return
	}

	response, err := gql.GetAddOn(ctx, client.GenqClient, databases[index].Name)
	if err != nil {
		return err
	}

	database := response.AddOn

	agentclient, err := agent.Establish(ctx, client)
	if err != nil {
		return err
	}

	dialer, err := agentclient.ConnectToTunnel(ctx, database.Organization.Slug)
	if err != nil {
		return err
	}

	localProxyPort := "16379"

	params := &proxy.ConnectParams{
		Ports:            []string{localProxyPort, "6379"},
		OrganizationSlug: database.Organization.Slug,
		Dialer:           dialer,
		RemoteHost:       database.PrivateIp,
	}

	go proxy.Connect(ctx, params)

	redisCliPath, err := exec.LookPath("redis-cli")
	if err != nil {
		fmt.Fprintf(io.Out, "Could not find redis-cli in your $PATH. Install it or point your redis-cli at: %s", "someurl")
	} else {
		// TODO: let proxy.Connect inform us about readiness
		time.Sleep(3 * time.Second)
		cmd := exec.CommandContext(ctx, redisCliPath, "-p", localProxyPort)
		cmd.Env = append(cmd.Env, fmt.Sprintf("REDISCLI_AUTH=%s", database.Password))
		cmd.Stdout = io.Out
		cmd.Stderr = io.ErrOut
		cmd.Stdin = io.In

		cmd.Start()
		cmd.Wait()
	}

	return
}
