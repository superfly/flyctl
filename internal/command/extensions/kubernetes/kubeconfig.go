package kubernetes

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/gql"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flyutil"
)

func saveKubeconfig() (cmd *cobra.Command) {
	const (
		short = "Save a Kubernetes cluster's kubeconfig to a file"
		long  = `Fetch the kubeconfig for the cluster named in the argument and write it
to ` + "`<cluster-name>.kubeconfig.yml`" + ` in the current directory, or to the
path given with ` + "`--output`" + `. The file is readable only by you and
replaces any existing file at that path.`
		usage = "save-kubeconfig [cluster name]"
	)

	cmd = command.New(usage, short, long, runSaveKubeconfig, command.RequireSession)
	cmd.Args = cobra.ExactArgs(1)
	cmd.Hidden = false
	flag.Add(cmd,
		flag.String{
			Name:        "output",
			Description: "The output path to save the kubeconfig file",
		},
	)

	return cmd
}

func runSaveKubeconfig(ctx context.Context) error {
	client := flyutil.ClientFromContext(ctx).GenqClient()
	clusterName := flag.FirstArg(ctx)

	resp, err := gql.GetAddOn(ctx, client, clusterName, string(gql.AddOnTypeKubernetes))
	if err != nil {
		return err
	}

	metadata := resp.AddOn.Metadata.(map[string]any)
	kubeconfig, ok := metadata["kubeconfig"].(string)
	if !ok {
		return fmt.Errorf("Failed to fetch kubeconfig. If provisioning your cluster failed you may have to delete it and reprovision it.")
	}

	outFilename := flag.GetString(ctx, "output")
	if outFilename == "" {
		outFilename = fmt.Sprintf("%s.kubeconfig.yml", resp.AddOn.Name)
	}

	return writeKubeconfig(outFilename, kubeconfig)
}
