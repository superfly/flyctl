package kubernetes

import (
	"context"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/command"
	extensions_core "github.com/superfly/flyctl/internal/command/extensions/core"
	"github.com/superfly/flyctl/internal/command/orgs"
	"github.com/superfly/flyctl/internal/flag"
)

func create() (cmd *cobra.Command) {

	const (
		short = "Provision a Kubernetes cluster for an organization"
		long  = short + "\n"
		usage = "create [organization slug]"
	)

	cmd = command.New(usage, short, long, runK8sCreate, command.RequireSession)
	cmd.Args = cobra.MaximumNArgs(1)
	flag.Add(cmd,
		flag.String{
			Name:        "name",
			Shorthand:   "n",
			Description: "The name of your cluster",
		},
	)
	return cmd
}

func runK8sCreate(ctx context.Context) (err error) {
	targetOrg, err := orgs.OrgFromEnvVarOrFirstArgOrSelect(ctx)
	if err != nil {
		return err
	}
	_, err = extensions_core.ProvisionExtension(ctx, extensions_core.ExtensionParams{
		Provider:     "kubernetes",
		Organization: targetOrg,
	})
	if err != nil {
		return err
	}
	return
}
