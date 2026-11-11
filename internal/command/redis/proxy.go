package redis

import (
	"context"
	"errors"
	"fmt"

	genq "github.com/Khan/genqlient/graphql"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/agent"
	"github.com/superfly/flyctl/gql"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flag/flagnames"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/prompt"
	"github.com/superfly/flyctl/internal/uiexutil"
	"github.com/superfly/flyctl/proxy"
	"github.com/superfly/flyctl/terminal"
)

func newProxy() (cmd *cobra.Command) {
	const (
		short = "Proxy to a Redis database"
		long  = short + `. Pass the database name to skip the interactive picker,
for example in scripts. Without a name, you choose from the Redis databases you
can access, narrowed by --org if set.`

		usage = "proxy [name]"
	)

	cmd = command.New(usage, short, long, runProxy, command.RequireSession)
	cmd.Args = cobra.MaximumNArgs(1)

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
	name, orgSlug := redisTarget(ctx)

	database, err := resolveRedisDatabase(ctx, client.GenqClient(), name, orgSlug,
		func(ctx context.Context, orgSlug string) error {
			_, err := uiexutil.ClientFromContext(ctx).GetOrganization(ctx, orgSlug)
			return err
		},
		func(ctx context.Context, options []string) (int, error) {
			var index int
			err := prompt.Select(ctx, &index, "Select a database to connect to", "", options...)
			return index, err
		},
	)
	if err != nil {
		return nil, "", err
	}

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

// redisTarget returns the database name argument and the org to scope by.
// FLY_ORG still narrows the interactive picker, as before, but a database the
// user names explicitly is only checked against an org passed with --org: an
// ambient default org shouldn't reject a database the user asked for by name.
func redisTarget(ctx context.Context) (name, orgSlug string) {
	name = flag.FirstArg(ctx)
	if name != "" && !flag.IsSpecified(ctx, flagnames.Org) {
		return name, ""
	}

	return name, flag.GetOrg(ctx)
}

// resolveRedisDatabase finds the Redis database to connect to.
//
// With a name it looks that database up directly, with no listing and no
// prompt. If orgSlug is set, the org must exist and own the database.
//
// Without a name it lists the databases (scoped to orgSlug when set) and asks
// choose to pick one. The list is already scoped, so no ownership check runs.
func resolveRedisDatabase(
	ctx context.Context,
	client genq.Client,
	name, orgSlug string,
	checkOrg func(ctx context.Context, orgSlug string) error,
	choose func(ctx context.Context, options []string) (int, error),
) (*gql.GetAddOnAddOn, error) {
	if name == "" {
		picked, err := pickRedisDatabase(ctx, client, orgSlug, checkOrg, choose)
		if err != nil {
			return nil, err
		}

		return getRedisDatabase(ctx, client, picked)
	}

	var org *gql.GetOrganizationOrganization
	if orgSlug != "" {
		resp, err := gql.GetOrganization(ctx, client, orgSlug)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve organization %q: %w", orgSlug, err)
		}
		org = &resp.Organization
	}

	database, err := getRedisDatabase(ctx, client, name)
	if err != nil {
		return nil, err
	}

	// An org can be addressed by its slug or its raw slug (personal orgs are
	// aliased as "personal"), so accept either.
	if org != nil && database.Organization.Slug != org.Slug && database.Organization.Slug != org.RawSlug {
		return nil, fmt.Errorf("Redis database %q belongs to organization %q, not %q (from --org)", name, database.Organization.Slug, orgSlug)
	}

	return database, nil
}

func getRedisDatabase(ctx context.Context, client genq.Client, name string) (*gql.GetAddOnAddOn, error) {
	response, err := gql.GetAddOn(ctx, client, name, string(gql.AddOnTypeUpstashRedis))
	if err != nil {
		return nil, err
	}

	return &response.AddOn, nil
}

func pickRedisDatabase(
	ctx context.Context,
	client genq.Client,
	orgSlug string,
	checkOrg func(ctx context.Context, orgSlug string) error,
	choose func(ctx context.Context, options []string) (int, error),
) (string, error) {
	var options, names []string

	if orgSlug != "" {
		if err := checkOrg(ctx, orgSlug); err != nil {
			return "", err
		}

		result, err := gql.ListOrganizationAddOns(ctx, client, orgSlug, "upstash_redis")
		if err != nil {
			return "", err
		}

		for _, database := range result.Organization.AddOns.Nodes {
			names = append(names, database.Name)
			options = append(options, fmt.Sprintf("%s (%s) %s", database.Name, database.PrimaryRegion, orgSlug))
		}
	} else {
		result, err := gql.ListAddOns(ctx, client, "upstash_redis")
		if err != nil {
			return "", err
		}

		for _, database := range result.AddOns.Nodes {
			names = append(names, database.Name)
			options = append(options, fmt.Sprintf("%s (%s) %s", database.Name, database.PrimaryRegion, database.Organization.Slug))
		}
	}

	if len(names) == 0 {
		return "", errors.New("no Redis databases found; create one with `fly redis create`")
	}

	index, err := choose(ctx, options)
	switch {
	case prompt.IsNonInteractive(err):
		return "", prompt.NonInteractiveError("pass the Redis database name as an argument when not running interactively")
	case err != nil:
		return "", err
	}

	return names[index], nil
}
