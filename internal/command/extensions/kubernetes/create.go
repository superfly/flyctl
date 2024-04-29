package kubernetes

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/gql"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command"
	extensions_core "github.com/superfly/flyctl/internal/command/extensions/core"
	"github.com/superfly/flyctl/internal/command/orgs"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/iostreams"
)

const (
	betaMsg = "Fly Kubernetes is in beta, it is not recommended for critical production use cases. For help or feedback, email us at beta@fly.io"
)

func create() (cmd *cobra.Command) {

	const (
		short = "Provision a Kubernetes cluster for an organization"
		long  = short + "\n"
		usage = "create [flags]"
	)

	cmd = command.New(usage, short, long, runK8sCreate, command.RequireSession)
	flag.Add(cmd,
		flag.String{
			Name:        "name",
			Shorthand:   "n",
			Description: "The name of your cluster",
		},
		flag.Org(),
		flag.Region(),
	)
	return cmd
}

func runK8sCreate(ctx context.Context) (err error) {
	io := iostreams.FromContext(ctx)
	colorize := io.ColorScheme()
	fmt.Fprintln(io.Out, colorize.Yellow(betaMsg))

	client := fly.ClientFromContext(ctx).GenqClient
	appName := appconfig.NameFromContext(ctx)
	targetOrg, err := orgs.OrgFromFlagOrSelect(ctx)
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

	outFilename := fmt.Sprintf("%s.kubeconfig.yml", resp.AddOn.Name)
	f, err := os.Create(outFilename)
	if err != nil {
		return err
	}
	kubeconfig := metadata["kubeconfig"].(string)
	_, err = f.Write([]byte(kubeconfig))
	if err != nil {
		return fmt.Errorf("failed to write %s to disk, error: %w", outFilename, err)
	}

	fmt.Fprintf(io.Out, "Wrote kubeconfig to file %s. Use it to connect to your cluster", outFilename)
	return
}
