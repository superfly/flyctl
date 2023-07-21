package extensions_core

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/briandowns/spinner"
	"github.com/skratchdot/open-golang/open"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/gql"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command/secrets"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/prompt"
	"github.com/superfly/flyctl/iostreams"
	"golang.org/x/exp/slices"
)

type ExtensionOptions struct {
	Provider       string
	SelectName     bool
	SelectRegion   bool
	NameSuffix     string
	DetectPlatform bool
	Options        gql.AddOnOptions
}

func ProvisionExtension(ctx context.Context, options ExtensionOptions) (addOn *gql.AddOn, err error) {
	client := client.FromContext(ctx).API().GenqClient
	appName := appconfig.NameFromContext(ctx)
	io := iostreams.FromContext(ctx)
	colorize := io.ColorScheme()
	// Fetch the target organization from the app
	appResponse, err := gql.GetAppWithAddons(ctx, client, appName, gql.AddOnType(options.Provider))

	if err != nil {
		return nil, err
	}

	targetApp := appResponse.App.AppData
	targetOrg := targetApp.Organization
	resp, err := gql.GetAddOnProvider(ctx, client, options.Provider)

	if err != nil {
		return nil, err
	}

	addOnProvider := resp.AddOnProvider

	tosResp, err := gql.AgreedToProviderTos(ctx, client, options.Provider, targetOrg.Id)

	if err != nil {
		return nil, err
	}

	if !tosResp.Organization.AgreedToProviderTos {
		if err != nil {
			return nil, err
		}

		confirmTos, err := prompt.Confirm(ctx, fmt.Sprintf("To continue, your organization must agree to the %s Terms Of Service (%s). Do you agree?", addOnProvider.DisplayName, resp.AddOnProvider.TosUrl))

		if err != nil {
			return nil, err
		}

		if confirmTos {
			_, err := gql.CreateTosAgreement(ctx, client, gql.CreateExtensionTosAgreementInput{
				OrganizationId:    targetOrg.Id,
				AddOnProviderName: options.Provider,
			})

			if err != nil {
				return nil, err
			}
		} else {
			return nil, nil
		}
	}

	if len(appResponse.App.AddOns.Nodes) > 0 {
		errMsg := fmt.Sprintf("A %s extension named %s already exists for this app", addOnProvider.DisplayName, colorize.Green(appResponse.App.AddOns.Nodes[0].Name))
		return nil, errors.New(errMsg)
	}

	var name string

	if options.SelectName {
		name = flag.GetString(ctx, "name")

		if name == "" {
			if options.NameSuffix != "" {
				name = targetApp.Name + "-" + options.NameSuffix
			}
			err = prompt.String(ctx, &name, "Choose a name, use the default, or leave blank to generate one:", name, false)

			if err != nil {
				return nil, err
			}
		}
	} else {
		name = targetApp.Name
	}

	input := gql.CreateAddOnInput{
		OrganizationId: targetOrg.Id,
		Name:           name,
		AppId:          targetApp.Id,
		Type:           gql.AddOnType(options.Provider),
		Options:        options.Options,
	}

	if options.SelectRegion {

		var primaryRegion string

		excludedRegions, err := GetExcludedRegions(ctx, options.Provider)

		if err != nil {
			return addOn, err
		}

		cfg := appconfig.ConfigFromContext(ctx)

		if cfg != nil && cfg.PrimaryRegion != "" {

			primaryRegion = cfg.PrimaryRegion

			if slices.Contains(excludedRegions, primaryRegion) {
				fmt.Fprintf(io.ErrOut, "%s is only available in regions with low latency (<10ms) to Fly.io regions. That doesn't include '%s'.\n", addOnProvider.DisplayName, primaryRegion)

				confirm, err := prompt.Confirm(ctx, fmt.Sprintf("Would you like to provision anyway in the nearest region to '%s'?", primaryRegion))
				if err != nil || !confirm {
					return nil, err
				}
			}
		} else {

			region, err := prompt.Region(ctx, !targetOrg.PaidPlan, prompt.RegionParams{
				Message:             "Choose the primary region (can't be changed later)",
				ExcludedRegionCodes: excludedRegions,
			})

			if err != nil {
				return addOn, err
			}

			primaryRegion = region.Code
		}

		input.PrimaryRegion = primaryRegion
	}

	createAddOnResponse, err := gql.CreateAddOn(ctx, client, input)

	if err != nil {
		return nil, err
	}

	addOn = &createAddOnResponse.CreateAddOn.AddOn

	if addOnProvider.AsyncProvisioning {
		// wait for provision
		err = WaitForProvision(ctx, addOn.Name)
		if err != nil {
			return nil, err
		}
	}

	if options.SelectRegion {
		fmt.Fprintf(io.Out, "Created %s in the %s region for app %s\n\n", colorize.Green(addOn.Name), colorize.Green(addOn.PrimaryRegion), colorize.Green(appName))
	}
	fmt.Fprintf(io.Out, "Setting the following secrets on %s:\n", appName)

	env := make(map[string]string)
	for key, value := range addOn.Environment.(map[string]interface{}) {
		env[key] = value.(string)
		fmt.Println(key)
	}

	secrets.SetSecretsAndDeploy(ctx, gql.ToAppCompact(targetApp), env, false, false)

	return addOn, nil
}

func WaitForProvision(ctx context.Context, name string) error {
	io := iostreams.FromContext(ctx)
	client := client.FromContext(ctx).API().GenqClient

	s := spinner.New(spinner.CharSets[9], 200*time.Millisecond)
	s.Writer = io.ErrOut
	s.Prefix = "Waiting for provisioning to complete "
	s.Start()

	defer s.Stop()
	timeout := time.After(4 * time.Minute)
	ticker := time.NewTicker(2 * time.Second)

	defer ticker.Stop()

	for {

		resp, err := gql.GetAddOn(ctx, client, name)

		if err != nil {
			return err
		}

		if resp.AddOn.Status == "ready" {
			return nil
		}

		select {
		case <-ticker.C:
		case <-timeout:
			return errors.New("timed out waiting for provisioning to complete")
		case <-ctx.Done():
			return nil
		}
	}
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

func Discover(ctx context.Context, provider gql.AddOnType) (addOn *gql.AddOnData, app *gql.AppData, err error) {
	client := client.FromContext(ctx).API().GenqClient
	appName := appconfig.NameFromContext(ctx)

	if len(flag.Args(ctx)) == 1 {

		response, err := gql.GetAddOn(ctx, client, flag.FirstArg(ctx))
		if err != nil {
			return nil, nil, err
		}

		addOn = &response.AddOn.AddOnData

	} else if appName != "" {
		resp, err := gql.GetAppWithAddons(ctx, client, appName, provider)

		if err != nil {
			return nil, nil, err
		}

		addOn = &resp.App.AddOns.Nodes[0].AddOnData
		app = &resp.App.AppData
	} else {
		return nil, nil, errors.New("Run this command in a Fly app directory or pass a database name as the first argument.")
	}

	return
}
