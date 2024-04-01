package kubernetes

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/gql"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
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

	return cmd
}

func runSaveKubeconfig(ctx context.Context) error {
	client := fly.ClientFromContext(ctx).GenqClient
	clusterName := flag.FirstArg(ctx)

	resp, err := gql.GetAddOn(ctx, client, clusterName)
	if err != nil {
		return err
	}

	metadata := resp.AddOn.Metadata.(map[string]interface{})
	kubeconfig := metadata["kubeconfig"].(string)

	f, err := os.Create("kubeconfig")
	if err != nil {
		return fmt.Errorf("could not create kubeconfig file: %w", err)
	}
	defer f.Close()

	_, err = f.Write([]byte(kubeconfig))
	if err != nil {
		return fmt.Errorf("could not save kubeconfig: %w", err)
	}

	return nil
}
