package fly_mysql

import (
	"context"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/gql"
	"github.com/superfly/flyctl/internal/command"
	extensions_core "github.com/superfly/flyctl/internal/command/extensions/core"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flyutil"
)

func update() (cmd *cobra.Command) {
	const (
		short = "Update an existing MySQL database"
		long  = short + "\n"
	)

	cmd = command.New("update <database_name>", short, long, runUpdate, command.RequireSession, command.LoadAppNameIfPresent)
	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
		flag.Org(),
		extensions_core.SharedFlags,
		SharedFlags,
	)
	return cmd
}

func runUpdate(ctx context.Context) (err error) {
	client := flyutil.ClientFromContext(ctx).GenqClient()

	id := flag.FirstArg(ctx)
	response, err := gql.GetAddOn(ctx, client, id, string(gql.AddOnTypeFlyMysql))
	if err != nil {
		return
	}
	addOn := response.AddOn

	options, _ := addOn.Options.(map[string]interface{})

	_, err = gql.UpdateAddOn(ctx, client, addOn.Id, addOn.AddOnPlan.Id, []string{}, optionsFromFlags(ctx, options), addOn.Metadata)
	if err != nil {
		return
	}

	err = runStatus(ctx)
	return err
}
