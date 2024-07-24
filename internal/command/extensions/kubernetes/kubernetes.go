package kubernetes

import (
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/command"
)

func New() (cmd *cobra.Command) {

	const (
		short = "Provision and manage Kubernetes clusters"
		long  = short + "\n"
	)

	cmd = command.New("kubernetes", short, long, nil)
	cmd.Aliases = []string{"k8s"}
	cmd.AddCommand(create(), destroy(), list(), kubectlToken(), saveKubeconfig())
	cmd.Hidden = false
	return cmd
}
