package extensions_core

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"slices"
	"sort"
	"time"

	"github.com/briandowns/spinner"
	"github.com/samber/lo"
	"github.com/skratchdot/open-golang/open"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/gql"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/prompt"
	"github.com/superfly/flyctl/internal/render"
	"github.com/superfly/flyctl/iostreams"
	"github.com/superfly/flyctl/scanner"
)

type Extension struct {
	Data        gql.ExtensionData
	App         *gql.AppData
	SetsSecrets bool
}

type ExtensionParams struct {
	AppName              string
	Organization         *fly.Organization
	Provider             string
	PlanID               string
	OrganizationPlanID   string
	Options              map[string]interface{}
	ErrorCaptureCallback func(ctx context.Context, provisioningError error, params *ExtensionParams) error

	// Surely there's a nicer way to do this, but this gets `fly launch` unblocked on launching exts
	OverrideRegion                  string
	OverrideName                    *string
	OverrideExtensionSecretKeyNames map[string]map[string]string
}

// Common flags that should be used for all extension commands
var SharedFlags = flag.Set{
	flag.Yes(),
}

func ProvisionExtension(ctx context.Context, params ExtensionParams) (extension Extension, err error) {
	client := flyutil.ClientFromContext(ctx).GenqClient()
	io := iostreams.FromContext(ctx)
	colorize := io.ColorScheme()

	var targetApp gql.AppData
	var targetOrg gql.OrganizationData

	resp, err := gql.GetAddOnProvider(ctx, client, params.Provider)
	if err != nil {
		return
	}

	provider := resp.AddOnProvider.ExtensionProviderData

	// Ensure users have agreed to the provider terms of service
	err = AgreeToProviderTos(ctx, provider)
	if err != nil {
		return extension, err
	}

	if params.AppName != "" {
		appResponse, err := gql.GetAppWithAddons(ctx, client, params.AppName, gql.AddOnType(params.Provider))
		if err != nil {
			return extension, err
		}

		targetApp = appResponse.App.AppData
		targetOrg = appResponse.App.Organization.OrganizationData

		if len(appResponse.App.AddOns.Nodes) > 0 {
			existsError := fmt.Errorf("A %s %s named %s already exists for app %s", provider.DisplayName, provider.ResourceName, colorize.Green(appResponse.App.AddOns.Nodes[0].Name), colorize.Green(params.AppName))

			return extension, existsError
		}

	} else {
		resp, err := gql.GetOrganization(ctx, client, params.Organization.Slug)
		if err != nil {
			return extension, err
		}

		targetOrg = resp.Organization.OrganizationData
	}

	var name string

	// Prompt to name the provisioned resource, or use the target app name like in Sentry's case
	if provider.SelectName {
		if override := params.OverrideName; override != nil {
			name = *override
		} else {
			if name == "" {
				name = flag.GetString(ctx, "name")
			}

			if name == "" {
				if provider.NameSuffix != "" && targetApp.Name != "" {
					name = targetApp.Name + "-" + provider.NameSuffix
				}
				err = prompt.String(ctx, &name, "Choose a name, use the default, or leave blank to generate one:", name, false)
				if err != nil {
					return
				}
			}
		}
	} else {
		name = targetApp.Name
	}

	input := gql.CreateAddOnInput{
		OrganizationId: targetOrg.Id,
		Name:           name,
		AppId:          targetApp.Id,
		Type:           gql.AddOnType(provider.Name),
	}

	if params.PlanID != "" {
		input.PlanId = params.PlanID
	}

	if params.OrganizationPlanID != "" {
		input.OrganizationPlanId = params.OrganizationPlanID
	}

	var inExcludedRegion bool
	var primaryRegion string

	if provider.SelectRegion {

		excludedRegions, err := GetExcludedRegions(ctx, provider)
		if err != nil {
			return extension, err
		}

		desiredRegion := params.OverrideRegion
		if desiredRegion != "" {
			cfg := appconfig.ConfigFromContext(ctx)
			if cfg != nil && cfg.PrimaryRegion != "" {
				desiredRegion = cfg.PrimaryRegion
			}
		}

		if desiredRegion != "" {

			primaryRegion = desiredRegion

			if slices.Contains(excludedRegions, primaryRegion) {
				inExcludedRegion = true
			}

		} else {

			region, err := prompt.Region(ctx, !targetOrg.PaidPlan, prompt.RegionParams{
				Message:             "Choose the primary region (can't be changed later)",
				ExcludedRegionCodes: excludedRegions,
			})
			if err != nil {
				return extension, err
			}

			primaryRegion = region.Code
		}

		input.PrimaryRegion = primaryRegion
	}

	var detectedPlatform *scanner.SourceInfo

	// Pass the detected platform family to the API
	if provider.DetectPlatform {
		absDir, err := filepath.Abs(".")
		if err != nil {
			return extension, err
		}

		detectedPlatform, err = scanner.Scan(absDir, &scanner.ScannerConfig{Colorize: io.ColorScheme()})
		if err != nil {
			return extension, err
		}

		if detectedPlatform != nil && PlatformMap[detectedPlatform.Family] != "" {
			if params.Options == nil {
				params.Options = gql.AddOnOptions{}
			}
			params.Options["platform"] = PlatformMap[detectedPlatform.Family]
		}
	}

	input.Options = params.Options
	createResp, err := gql.CreateExtension(ctx, client, input)

	if params.ErrorCaptureCallback != nil {
		err = params.ErrorCaptureCallback(ctx, err, &params)
	}

	if err != nil {
		return
	}

	extension.Data = createResp.CreateAddOn.AddOn.ExtensionData
	extension.App = &targetApp

	if provider.AsyncProvisioning {
		err = WaitForProvision(ctx, extension.Data.Name, params.Provider)
		if err != nil {
			return
		}
	}

	// Display provisioning notification to user

	provisioningMsg := fmt.Sprintf("Your %s %s", provider.DisplayName, provider.ResourceName)

	if provider.SelectName {
		provisioningMsg = provisioningMsg + fmt.Sprintf(" (%s)", colorize.Green(extension.Data.Name))
	}

	if provider.SelectRegion {
		provisioningMsg = provisioningMsg + fmt.Sprintf(" in %s", colorize.Green(extension.Data.PrimaryRegion))
	}

	fmt.Fprintf(io.Out, provisioningMsg+" is ready. See details and next steps with: %s\n\n", colorize.Green(provider.ProvisioningInstructions))

	if inExcludedRegion {
		fmt.Fprintf(io.ErrOut,
			"Note: Your app is deployed in %s which isn't a supported %s region. Expect database request latency of 10ms or more.\n\n",
			colorize.Green(primaryRegion), provider.DisplayName)
	}

	// Also take into consideration custom key names to replace extension's default secret key names
	overrideSecretKeyNamesMap := params.OverrideExtensionSecretKeyNames[params.Provider]
	setSecretsFromExtension(ctx, &targetApp, &extension, overrideSecretKeyNamesMap)

	return extension, nil
}

func AgreeToProviderTos(ctx context.Context, provider gql.ExtensionProviderData) error {
	client := flyutil.ClientFromContext(ctx).GenqClient()
	out := iostreams.FromContext(ctx).Out

	// Internal providers like kubernetes don't need ToS agreement
	if provider.Internal {
		return nil
	}

	// Check if the provider ToS was agreed to already
	agreed, err := AgreedToProviderTos(ctx, provider.Name)
	if err != nil {
		return err
	}

	if agreed {
		return nil
	}

	fmt.Fprint(out, "\n"+provider.TosAgreement+"\n\n")

	// Prompt the user to agree to the provider ToS, or display the ToS agreement copy
	// for non-interactive sessions
	if !flag.GetYes(ctx) {
		switch confirmTos, err := prompt.Confirm(ctx, "Do you agree?"); {
		case err == nil:
			if !confirmTos {
				return errors.New("You must agree to continue.")
			}
		case prompt.IsNonInteractive(err):
			return prompt.NonInteractiveError("To agree, the --yes flag must be specified when not running interactively")
		default:
			return err
		}
	} else {
		fmt.Fprintln(out, "By specifying the --yes flag, you have agreed to the terms displayed above.")
		return nil
	}

	_, err = gql.CreateTosAgreement(ctx, client, provider.Name)

	return err
}

func WaitForProvision(ctx context.Context, name string, provider string) error {
	io := iostreams.FromContext(ctx)
	client := flyutil.ClientFromContext(ctx).GenqClient()

	s := spinner.New(spinner.CharSets[9], 200*time.Millisecond)
	s.Writer = io.ErrOut
	s.Prefix = "Waiting for provisioning to complete "
	s.Start()

	defer s.Stop()
	timeout := time.After(4 * time.Minute)
	ticker := time.NewTicker(2 * time.Second)

	defer ticker.Stop()

	for {

		resp, err := gql.GetAddOn(ctx, client, name, provider)
		if err != nil {
			return err
		}

		if resp.AddOn.Status == "error" {
			if resp.AddOn.ErrorMessage != "" {
				return errors.New(resp.AddOn.ErrorMessage)
			} else {
				return errors.New("provisioning failed")
			}
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

func GetExcludedRegions(ctx context.Context, provider gql.ExtensionProviderData) (excludedRegions []string, err error) {
	for _, region := range provider.ExcludedRegions {
		excludedRegions = append(excludedRegions, region.Code)
	}

	return
}

func OpenOrgDashboard(ctx context.Context, orgSlug string, providerName string) (err error) {
	client := flyutil.ClientFromContext(ctx).GenqClient()

	resp, err := gql.GetAddOnProvider(ctx, client, providerName)
	if err != nil {
		return
	}

	provider := resp.AddOnProvider.ExtensionProviderData

	err = AgreeToProviderTos(ctx, provider)
	if err != nil {
		return err
	}

	result, err := gql.GetExtensionSsoLink(ctx, client, orgSlug, providerName)
	if err != nil {
		return err
	}

	err = openUrl(ctx, result.Organization.ExtensionSsoLink)

	return
}

func openUrl(ctx context.Context, url string) (err error) {
	io := iostreams.FromContext(ctx)

	fmt.Fprintf(io.Out, "Opening %s ...\n", url)

	if err := open.Run(url); err != nil {
		return fmt.Errorf("failed opening %s: %w", url, err)
	}
	return
}

func OpenDashboard(ctx context.Context, extensionName string, provider gql.AddOnType) (err error) {
	client := flyutil.ClientFromContext(ctx).GenqClient()

	result, err := gql.GetAddOn(ctx, client, extensionName, string(provider))
	if err != nil {
		return err
	}

	err = AgreeToProviderTos(ctx, result.AddOn.AddOnProvider.ExtensionProviderData)
	if err != nil {
		return err
	}

	url := result.AddOn.SsoLink
	openUrl(ctx, url)

	return
}

func Discover(ctx context.Context, provider gql.AddOnType) (addOn *gql.AddOnData, app *gql.AppData, err error) {
	client := flyutil.ClientFromContext(ctx).GenqClient()
	appName := appconfig.NameFromContext(ctx)

	if len(flag.Args(ctx)) == 1 {
		response, err := gql.GetAddOn(ctx, client, flag.FirstArg(ctx), string(provider))
		if err != nil {
			return nil, nil, err
		}

		addOn = &response.AddOn.AddOnData

	} else if appName != "" {
		resp, err := gql.GetAppWithAddons(ctx, client, appName, provider)
		if err != nil {
			return nil, nil, err
		}

		if len(resp.App.AddOns.Nodes) == 0 {
			return nil, nil, fmt.Errorf("No project found. Provision one with 'flyctl ext %s create'.", provider)
		}

		addOn = &resp.App.AddOns.Nodes[0].AddOnData
		app = &resp.App.AppData
	} else {
		return nil, nil, errors.New("Run this command in a Fly app directory or pass a name as the first argument.")
	}

	return
}

func setSecretsFromExtension(ctx context.Context, app *gql.AppData, extension *Extension, overrideSecretKeyNamesMap map[string]string) (err error) {
	var (
		io              = iostreams.FromContext(ctx)
		client          = flyutil.ClientFromContext(ctx).GenqClient()
		setSecrets bool = true
	)

	environment := extension.Data.Environment

	if environment == nil || reflect.ValueOf(environment).IsNil() {
		return nil
	}

	secrets := extension.Data.Environment.(map[string]interface{})

	if app.Name != "" {
		appResp, err := gql.GetApp(ctx, client, app.Name)
		if err != nil {
			return err
		}

		var matchingNames []string

		for _, s := range appResp.App.Secrets {
			if _, exists := secrets[s.Name]; exists {
				matchingNames = append(matchingNames, s.Name)
			}
		}

		if len(matchingNames) > 0 {
			fmt.Fprintf(io.Out, "Secrets %v already exist on app %s. They won't be set automatically.\n\n", matchingNames, app.Name)
			setSecrets = false
		}

	} else {
		setSecrets = false
	}

	keys := lo.Keys(secrets)
	sort.Strings(keys)

	if setSecrets {
		extension.SetsSecrets = true
		fmt.Fprintf(io.Out, "Setting the following secrets on %s:\n", app.Name)
		input := gql.SetSecretsInput{
			AppId: app.Id,
		}
		for _, key := range keys {
			if customKeyName, exists := overrideSecretKeyNamesMap[key]; exists {
				// If a custom key name is identified for the extension's secret key, use that custom key
				input.Secrets = append(input.Secrets, gql.SecretInput{Key: customKeyName, Value: secrets[key].(string)})
				fmt.Fprintf(io.Out, "%s: %s\n", customKeyName, secrets[key].(string))
			} else {
				// Use the default secret key name
				input.Secrets = append(input.Secrets, gql.SecretInput{Key: key, Value: secrets[key].(string)})
				fmt.Fprintf(io.Out, "%s: %s\n", key, secrets[key].(string))
			}
		}

		fmt.Fprintln(io.Out)

		_, err = gql.SetSecrets(ctx, client, input)
		if err != nil {
			return err
		}
	} else {
		fmt.Fprintf(io.Out, "Set the following secrets on your target app.\n")

		for _, key := range keys {
			fmt.Fprintf(io.Out, "%s: %s\n", key, secrets[key].(string))
		}

	}
	return err
}

func AgreedToProviderTos(ctx context.Context, providerName string) (bool, error) {
	client := flyutil.ClientFromContext(ctx).GenqClient()

	tosResp, err := gql.AgreedToProviderTos(ctx, client, providerName)
	if err != nil {
		return false, err
	}

	viewerUser, ok := tosResp.Viewer.(*gql.AgreedToProviderTosViewerUser)
	if ok {
		return viewerUser.AgreedToProviderTos, nil
	} else {
		// If we are unable to determine if the user has agreed to the provider ToS, return false
		return false, nil
	}
}

func Status(ctx context.Context, provider gql.AddOnType) (err error) {
	io := iostreams.FromContext(ctx)

	extension, app, err := Discover(ctx, provider)
	if err != nil {
		return err
	}

	obj := [][]string{
		{
			extension.Name,
			extension.PrimaryRegion,
			extension.Status,
		},
	}

	var cols []string = []string{"Name", "Primary Region", "Status"}

	if app != nil {
		obj[0] = append(obj[0], app.Name)
		cols = append(cols, "App")
	}

	if err = render.VerticalTable(io.Out, "Status", obj, cols...); err != nil {
		return
	}

	return
}

// Supported Sentry Platforms from https://github.com/getsentry/sentry/blob/master/src/sentry/utils/platform_categories.py
// If other extensions require platform detection, this list can be abstracted further
var Platforms = []string{
	"android",
	"apple-ios",
	"apple-macos",
	"capacitor",
	"cocoa-objc",
	"cocoa-swift",
	"cordova",
	"csharp",
	"dart",
	"dart-flutter",
	"dotnet",
	"dotnet-aspnet",
	"dotnet-aspnetcore",
	"dotnet-awslambda",
	"dotnet-gcpfunctions",
	"dotnet-maui",
	"dotnet-winforms",
	"dotnet-wpf",
	"dotnet-xamarin",
	"electron",
	"elixir",
	"flutter",
	"go",
	"go-http",
	"ionic",
	"java",
	"java-android",
	"java-appengine",
	"java-log4j",
	"java-log4j2",
	"java-logback",
	"java-logging",
	"java-spring",
	"java-spring-boot",
	"javascript",
	"javascript-angular",
	"javascript-angularjs",
	"javascript-backbone",
	"javascript-capacitor",
	"javascript-cordova",
	"javascript-electron",
	"javascript-ember",
	"javascript-gatsby",
	"javascript-nextjs",
	"javascript-react",
	"javascript-remix",
	"javascript-svelte",
	"javascript-vue",
	"kotlin",
	"minidump",
	"native",
	"native-breakpad",
	"native-crashpad",
	"native-minidump",
	"native-qt",
	"node",
	"node-awslambda",
	"node-azurefunctions",
	"node-connect",
	"node-express",
	"node-gcpfunctions",
	"node-koa",
	"perl",
	"php",
	"php-laravel",
	"php-monolog",
	"php-symfony2",
	"python",
	"python-awslambda",
	"python-azurefunctions",
	"python-bottle",
	"python-celery",
	"python-django",
	"python-fastapi",
	"python-flask",
	"python-gcpfunctions",
	"python-pylons",
	"python-pyramid",
	"python-rq",
	"python-sanic",
	"python-starlette",
	"python-tornado",
	"react-native",
	"ruby",
	"ruby-rack",
	"ruby-rails",
	"rust",
	"unity",
	"unreal",
}

var PlatformMap = map[string]string{
	"AdonisJS":      "node",
	"Bun":           "javascript",
	"Django":        "python-django",
	"Deno":          "node",
	".NET":          "dotnet",
	"Elixir":        "elixir",
	"Gatsby":        "node",
	"Go":            "go",
	"NodeJS":        "node",
	"NodeJS/Prisma": "node",
	"Laravel":       "php-laravel",
	"Meteor":        "javascript-meteor",
	"NestJS":        "node",
	"NextJS":        "javascript-nextjs",
	"Nuxt":          "javascript-vue",
	"NuxtJS":        "javascript-vue",
	"Phoenix":       "elixir",
	"Python":        "python",
	"Rails":         "ruby-rails",
	"RedwoodJS":     "javascript-react",
	"Remix":         "javascript-remix",
	"Remix/Prisma":  "javascript-remix",
	"Ruby":          "ruby",
	"Shopify":       "javascript-shopify",
}
