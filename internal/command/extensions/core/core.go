package extensions_core

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"github.com/briandowns/spinner"
	"github.com/skratchdot/open-golang/open"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/gql"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/prompt"
	"github.com/superfly/flyctl/iostreams"
	"github.com/superfly/flyctl/scanner"
	"golang.org/x/exp/slices"
)

type Extension struct {
	Data gql.ExtensionData
	App  gql.AppData
}

func ProvisionExtension(ctx context.Context, appName string, providerName string, auto bool, options map[string]interface{}) (extension Extension, err error) {
	client := client.FromContext(ctx).API().GenqClient
	io := iostreams.FromContext(ctx)
	colorize := io.ColorScheme()
	// Fetch the target organization from the app
	appResponse, err := gql.GetAppWithAddons(ctx, client, appName, gql.AddOnType(providerName))

	if err != nil {
		return
	}

	targetApp := appResponse.App.AppData
	targetOrg := targetApp.Organization
	resp, err := gql.GetAddOnProvider(ctx, client, providerName)

	if err != nil {
		return
	}

	provider := resp.AddOnProvider.ExtensionProviderData

	// Stop provisioning if being provisioned automatically, but the provider has auto-provisioning disabled

	if auto && !provider.AutoProvision {
		return extension, nil
	}

	// Stop provisioning if this app already has an extension of this type, but only display an error for
	// extensions that weren't automatically provisioned

	if len(appResponse.App.AddOns.Nodes) > 0 {
		existsError := fmt.Errorf("A %s %s named %s already exists for app %s", provider.DisplayName, provider.ResourceName, colorize.Green(appResponse.App.AddOns.Nodes[0].Name), colorize.Green(appName))

		if auto {
			existsError = nil
		}

		return extension, existsError
	}

	// Display ToS implicit agreement only once per org. Viewing makes the agreement official.

	err = DisplayTosAgreement(ctx, provider, targetOrg, auto)

	if err != nil {
		return extension, err
	}

	// Pick a name for the extension unless we want it to be the same as the app, like in Sentry's case
	var name string

	if provider.SelectName {
		name = flag.GetString(ctx, "name")

		if name == "" {
			if provider.NameSuffix != "" {
				name = targetApp.Name + "-" + provider.NameSuffix
			}
			err = prompt.String(ctx, &name, "Choose a name, use the default, or leave blank to generate one:", name, false)

			if err != nil {
				return
			}
		}
	} else {
		name = targetApp.Name
	}

	input := gql.CreateAddOnInput{
		OrganizationId: targetOrg.Id,
		Name:           name,
		AppId:          targetApp.Id,
		Type:           gql.AddOnType(providerName),
	}

	var inExcludedRegion bool
	var primaryRegion string

	if provider.SelectRegion {

		excludedRegions, err := GetExcludedRegions(ctx, provider)

		if err != nil {
			return extension, err
		}

		cfg := appconfig.ConfigFromContext(ctx)

		if cfg != nil && cfg.PrimaryRegion != "" {

			primaryRegion = cfg.PrimaryRegion

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

		detectedPlatform, err = scanner.Scan(absDir, &scanner.ScannerConfig{})

		if err != nil {
			return extension, err
		}

		if detectedPlatform != nil && PlatformMap[detectedPlatform.Family] != "" {
			if options == nil {
				options = gql.AddOnOptions{}
			}
			options["platform"] = PlatformMap[detectedPlatform.Family]
		}

	}

	input.Options = options
	createResp, err := gql.CreateExtension(ctx, client, input)

	if err != nil {
		return
	}

	extension.Data = createResp.CreateAddOn.AddOn.ExtensionData
	extension.App = targetApp

	if provider.AsyncProvisioning {
		err = WaitForProvision(ctx, extension.Data.Name)
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

	fmt.Fprintf(io.Out, provisioningMsg+" is ready. See details and next steps with `flyctl ext %s dashboard`\n\n", provider.Name)

	if inExcludedRegion {
		fmt.Fprintf(io.ErrOut,
			"Note: Your app is deployed in %s which isn't a supported %s region. Expect database request latency of 10ms or more.\n\n",
			colorize.Green(primaryRegion), provider.DisplayName)
	}

	SetSecrets(ctx, &targetApp, extension.Data.Environment.(map[string]interface{}))

	return extension, nil
}

func DisplayTosAgreement(ctx context.Context, provider gql.ExtensionProviderData, org gql.AppDataOrganization, auto bool) error {
	io := iostreams.FromContext(ctx)
	colorize := io.ColorScheme()
	client := client.FromContext(ctx).API().GenqClient

	agreed, err := AgreedToProviderTos(ctx, provider.Name, org.Id)

	if err != nil {
		return err
	}

	tosMessage := "you agree to the %s Terms of Service: https://fly.io/legal/terms-of-service/#supplementalterms"

	var tosMsgPrefix string

	// Display different ToS copy if an extension was automatically provisioned at deploy time
	if auto {
		tosMsgPrefix = "We'll setup free error tracking for your app on Sentry.io. By deploying this app"
	} else {
		tosMsgPrefix = fmt.Sprintf("By provisioning this %s", provider.ResourceName)
	}

	tosMessage = colorize.Green("* ") + tosMsgPrefix + ", " + tosMessage

	if !agreed {
		fmt.Fprintf(io.Out, tosMessage+"\n\n", provider.DisplayName)
		_, err = gql.CreateTosAgreement(ctx, client, gql.CreateExtensionTosAgreementInput{
			AddOnProviderName: provider.Name,
			OrganizationId:    org.Id,
		})
	}

	return err
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

func GetExcludedRegions(ctx context.Context, provider gql.ExtensionProviderData) (excludedRegions []string, err error) {

	for _, region := range provider.ExcludedRegions {
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

func SetSecrets(ctx context.Context, app *gql.AppData, secrets map[string]interface{}) error {
	var (
		io     = iostreams.FromContext(ctx)
		client = client.FromContext(ctx).API().GenqClient
	)

	input := gql.SetSecretsInput{
		AppId: app.Id,
	}

	fmt.Fprintf(io.Out, "Setting the following secrets on %s:\n", app.Name)

	for key, value := range secrets {
		input.Secrets = append(input.Secrets, gql.SecretInput{Key: key, Value: value.(string)})
		fmt.Println(key)
	}

	fmt.Fprintln(io.Out)

	_, err := gql.SetSecrets(ctx, client, input)

	return err
}

func AgreedToProviderTos(ctx context.Context, providerName string, orgId string) (bool, error) {
	client := client.FromContext(ctx).API().GenqClient

	tosResp, err := gql.AgreedToProviderTos(ctx, client, providerName, orgId)

	if err != nil {
		return false, err
	}

	return tosResp.Organization.AgreedToProviderTos, nil
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
	"NestJS":        "node",
	"NextJS":        "javascript-nextjs",
	"Nuxt":          "javascript-vue",
	"NuxtJS":        "javascript-vue",
	"Phoenix":       "elixir",
	"Python":        "python",
	"Rails":         "ruby-rails",
	"RedwoodJS":     "javascript-react",
	"Remix":         "javscript-remix",
	"Remix/Prisma":  "javscript-remix",
	"Ruby":          "ruby",
}
