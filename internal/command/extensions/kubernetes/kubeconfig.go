package kubernetes

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/gql"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flyutil"
)

func saveKubeconfig() (cmd *cobra.Command) {
	const (
		long  = `Save the kubeconfig file of your cluster`
		short = long
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

	metadata := resp.AddOn.Metadata.(map[string]interface{})
	kubeconfig, ok := metadata["kubeconfig"].(string)
	if !ok {
		return fmt.Errorf("Failed to fetch kubeconfig. If provisioning your cluster failed you may have to delete it and reprovision it.")
	}

	outFilename := flag.GetString(ctx, "output")
	if outFilename == "" {
		outFilename = fmt.Sprintf("%s.kubeconfig.yml", resp.AddOn.Name)
	}
	f, err := os.Create(outFilename)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = f.Write([]byte(kubeconfig))
	if err != nil {
		return fmt.Errorf("failed to write kubeconfig to file %s, error: %w", outFilename, err)
	}

	return nil
}
