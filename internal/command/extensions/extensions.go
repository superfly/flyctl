// Package logs implements the logs command chain.
package extensions

import (
	"context"
	"fmt"

	"github.com/skratchdot/open-golang/open"
	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/gql"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/command/secrets"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/prompt"
	"github.com/superfly/flyctl/iostreams"
)

func New() (cmd *cobra.Command) {
	const (
		long = `Extensions are additional functionality that can be added to your Fly apps`
	)

	cmd = command.New("extensions", long, long, nil)
	cmd.Aliases = []string{"extensions", "ext"}

	cmd.Args = cobra.NoArgs

	cmd.AddCommand(newSentry(), newPlanetscale())
	return
}

func ProvisionExtension(ctx context.Context, provider string) (addOn *gql.AddOn, err error) {
	client := client.FromContext(ctx).API().GenqClient
	appName := appconfig.NameFromContext(ctx)
	io := iostreams.FromContext(ctx)

	// Fetch the target organization from the app
	appResponse, err := gql.GetApp(ctx, client, appName)

	if err != nil {
		return nil, err
	}

	targetApp := appResponse.App.AppData
	targetOrg := targetApp.Organization

	var name = flag.GetString(ctx, "name")

	if name == "" {
		err = prompt.String(ctx, &name, "Choose a name (leave blank to generate one):", "", false)

		if err != nil {
			return nil, err
		}
	}

	_, err = gql.GetAddOn(ctx, client, appName)

	// TODO: Check for a not-found error instead of a general error
	if err != nil {

		excludedRegions, err := GetExcludedRegions(ctx, provider)

		if err != nil {
			return addOn, err
		}

		primaryRegion, err := prompt.Region(ctx, !targetOrg.PaidPlan, prompt.RegionParams{
			Message:             "Choose the primary region (can't be changed later)",
			ExcludedRegionCodes: excludedRegions,
		})

		if err != nil {
			return addOn, err
		}

		input := gql.CreateAddOnInput{
			OrganizationId: targetOrg.Id,
			Name:           appName,
			PrimaryRegion:  primaryRegion.Code,
			AppId:          targetApp.Id,
			Type:           gql.AddOnType(provider),
		}

		createAddOnResponse, err := gql.CreateAddOn(ctx, client, input)

		if err != nil {
			return nil, err
		}

		addOn := createAddOnResponse.CreateAddOn.AddOn
		fmt.Fprintf(io.Out, "Created %s for app %s\n", addOn.Name, appName)
		fmt.Fprintf(io.Out, "Setting the following secrets on %s:\n", appName)

		env := make(map[string]string)
		for key, value := range addOn.Environment.(map[string]interface{}) {
			env[key] = value.(string)
			fmt.Println(key)
		}

		secrets.SetSecretsAndDeploy(ctx, gql.ToAppCompact(targetApp), env, false, false)

		return &addOn, nil
	} else {
		fmt.Fprintln(io.Out, "A PlanetScale database already exists for this app")
	}

	return
}

func GetExcludedRegions(ctx context.Context, provider string) (excludedRegions []string, err error) {
	client := client.FromContext(ctx).API().GenqClient

	response, err := gql.GetAddOnProvider(ctx, client, provider)

	if err != nil {
		return nil, err
	}

	for _, region := range response.AddOnProvider.ExcludedRegions {
		excludedRegions = append(excludedRegions, region.Code)
	}

	return
}

func OpenDashboard(ctx context.Context, extensionName string) (err error) {
	var (
		io     = iostreams.FromContext(ctx)
		client = client.FromContext(ctx).API().GenqClient
	)

	result, err := gql.GetAddOn(ctx, client, extensionName)

	if err != nil {
		return err
	}

	url := result.AddOn.SsoLink
	fmt.Fprintf(io.Out, "Opening %s ...\n", url)

	if err := open.Run(url); err != nil {
		return fmt.Errorf("failed opening %s: %w", url, err)
	}

	return
}
