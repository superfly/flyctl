package kubernetes

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/gql"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command"
	extensions_core "github.com/superfly/flyctl/internal/command/extensions/core"
	"github.com/superfly/flyctl/internal/command/orgs"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/iostreams"
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
	io := iostreams.FromContext(ctx)
	client := client.FromContext(ctx).API().GenqClient
	appName := appconfig.NameFromContext(ctx)
	targetOrg, err := orgs.OrgFromEnvVarOrFirstArgOrSelect(ctx)
	if err != nil {
		return err
	}
	extension, err := extensions_core.ProvisionExtension(ctx, extensions_core.ExtensionParams{
		AppName:      appName,
		Provider:     "kubernetes",
		Organization: targetOrg,
	})
	if err != nil {
		return err
	}

	resp, err := gql.GetAddOn(ctx, client, extension.Data.Name)
	if err != nil {
		return err
	}

	metadata := resp.AddOn.Metadata.(map[string]interface{})

	fmt.Fprintf(io.Out, "Use the following kubeconfig to connect to your cluster:\n\n%s", metadata["kubeconfig"])
	return
}
