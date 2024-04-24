package enveloop

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/gql"
	"github.com/superfly/flyctl/internal/command"
	extensions_core "github.com/superfly/flyctl/internal/command/extensions/core"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/render"
	"github.com/superfly/flyctl/iostreams"
)

func plans() (cmd *cobra.Command) {
	const (
		long  = `List all Enveloop plans`
		short = long
		usage = "plans"
	)

	cmd = command.New(usage, short, long, runListPlans, command.RequireSession)
	cmd.Aliases = []string{"ls"}

	flag.Add(cmd,
		flag.Org(),
		extensions_core.SharedFlags,
	)
	return cmd
}

func runListPlans(ctx context.Context) (err error) {
	client := fly.ClientFromContext(ctx).GenqClient
	response, err := gql.ListAddOnPlans(ctx, client)
	if err != nil {
		return err
	}

	fmt.Printf("%+v\n", response)

	var rows [][]string
	for _, extension := range response.AddOnPlans.Nodes {
		rows = append(rows, []string{
			extension.Id,
			extension.DisplayName,
			extension.Description,
			fmt.Sprintf("$%d / mo", extension.PricePerMonth),
		})
	}

	out := iostreams.FromContext(ctx).Out
	_ = render.Table(out, "", rows, "Id", "Name", "Description", "Price")

	return nil
}
