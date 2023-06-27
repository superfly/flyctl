package extensions_core

import (
	"context"
	"errors"
	"fmt"

	"github.com/skratchdot/open-golang/open"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/gql"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command/secrets"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/prompt"
	"github.com/superfly/flyctl/iostreams"
)

func ProvisionExtension(ctx context.Context, provider string) (addOn *gql.AddOn, err error) {
	client := client.FromContext(ctx).API().GenqClient
	appName := appconfig.NameFromContext(ctx)
	io := iostreams.FromContext(ctx)
	colorize := io.ColorScheme()
	// Fetch the target organization from the app
	appResponse, err := gql.GetAppWithAddons(ctx, client, appName, gql.AddOnType(provider))

	if err != nil {
		return nil, err
	}

	targetApp := appResponse.App.AppData
	targetOrg := targetApp.Organization

	tosResp, err := gql.AgreedToProviderTos(ctx, client, provider, targetOrg.Id)

	if err != nil {
		return nil, err
	}

	if !tosResp.Organization.AgreedToProviderTos {
		confirmTos, err := prompt.Confirm(ctx, fmt.Sprintf("Your organization must agree to the %s Terms Of Service to continue. Do you agree?", provider))
		if err != nil {
			return nil, err
		}
		if confirmTos {

			_, err := gql.CreateTosAgreement(ctx, client, gql.CreateExtensionTosAgreementInput{
				OrganizationId:    targetOrg.Id,
				AddOnProviderName: provider,
			})

			if err != nil {
				return nil, err
			}
		} else {
			return nil, nil
		}
	}

	if len(appResponse.App.AddOns.Nodes) > 0 {
		errMsg := fmt.Sprintf("A PlanetScale database named %s already exists for this app", colorize.Green(appResponse.App.AddOns.Nodes[0].Name))
		return nil, errors.New(errMsg)
	}

	var name = flag.GetString(ctx, "name")

	if name == "" {
		err = prompt.String(ctx, &name, "Choose a name, use the default, or leave blank to generate one:", targetApp.Name+"-db", false)

		if err != nil {
			return nil, err
		}
	}

	excludedRegions, err := GetExcludedRegions(ctx, provider)

	if err != nil {
		return addOn, err
	}

	var primaryRegion string

	cfg := appconfig.ConfigFromContext(ctx)

	primaryRegion = cfg.PrimaryRegion

	if cfg.PrimaryRegion == "" {
		region, err := prompt.Region(ctx, !targetOrg.PaidPlan, prompt.RegionParams{
			Message:             "Choose the primary region (can't be changed later)",
			ExcludedRegionCodes: excludedRegions,
		})

		if err != nil {
			return addOn, err
		}

		primaryRegion = region.Code
	}

	input := gql.CreateAddOnInput{
		OrganizationId: targetOrg.Id,
		Name:           name,
		PrimaryRegion:  primaryRegion,
		AppId:          targetApp.Id,
		Type:           gql.AddOnType(provider),
	}

	createAddOnResponse, err := gql.CreateAddOn(ctx, client, input)

	if err != nil {
		return nil, err
	}

	addOn = &createAddOnResponse.CreateAddOn.AddOn
	fmt.Fprintf(io.Out, "Created %s in the %s region for app %s\n\n", colorize.Green(addOn.Name), colorize.Green(primaryRegion), colorize.Green(appName))
	fmt.Fprintf(io.Out, "Setting the following secrets on %s:\n", appName)

	env := make(map[string]string)
	for key, value := range addOn.Environment.(map[string]interface{}) {
		env[key] = value.(string)
		fmt.Println(key)
	}

	secrets.SetSecretsAndDeploy(ctx, gql.ToAppCompact(targetApp), env, false, false)

	return addOn, nil
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
