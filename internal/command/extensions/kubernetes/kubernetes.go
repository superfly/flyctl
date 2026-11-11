package kubernetes

import (
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/command"
)

func New() (cmd *cobra.Command) {

	const (
		short = "Provision and manage Fly Kubernetes clusters"
		long  = `Provision and manage Fly Kubernetes clusters, which are in beta. Create
a cluster with ` + "`fly ext k8s create`" + `, which writes a kubeconfig file for
kubectl. Fetch that file again later with ` + "`fly ext k8s save-kubeconfig`" + `.`
	)

	cmd = command.New("kubernetes", short, long, nil)
	cmd.Aliases = []string{"k8s"}
	cmd.AddCommand(create(), destroy(), list(), kubectlToken(), saveKubeconfig())
	cmd.Hidden = false

	return cmd
}
