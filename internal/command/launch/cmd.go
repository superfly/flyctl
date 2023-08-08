package launch

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/AlecAivazis/survey/v2"
	"github.com/samber/lo"
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/command/deploy"
	"github.com/superfly/flyctl/internal/command/launch/legacy"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/iostreams"
	"github.com/superfly/flyctl/scanner"
)

func New() (cmd *cobra.Command) {
	const (
		long  = `Create and configure a new app from source code or a Docker image.`
		short = long
	)

	cmd = command.New("launch", short, long, run, command.RequireSession, command.LoadAppConfigIfPresent)
	cmd.Args = cobra.NoArgs

	flag.Add(cmd,
		// Since launch can perform a deployment, we offer the full set of deployment flags for those using
		// the launch command in CI environments. We may want to rescind this decision down the line, because
		// the list of flags is long, but it follows from the precedent of already offering some deployment flags.
		// See a proposed 'flag grouping' feature in Viper that could help with DX: https://github.com/spf13/cobra/pull/1778
		deploy.CommonFlags,

		flag.Org(),
		flag.NoDeploy(),
		flag.Bool{
			Name:        "generate-name",
			Description: "Always generate a name for the app, without prompting",
		},
		flag.String{
			Name:        "path",
			Description: `Path to the app source root, where fly.toml file will be saved`,
			Default:     ".",
		},
		flag.String{
			Name:        "name",
			Description: `Name of the new app`,
		},
		flag.Bool{
			Name:        "copy-config",
			Description: "Use the configuration file if present without prompting",
			Default:     false,
		},
		flag.Bool{
			Name:        "reuse-app",
			Description: "Continue even if app name clashes with an existent app",
			Default:     false,
		},
		flag.Bool{
			Name:        "dockerignore-from-gitignore",
			Description: "If a .dockerignore does not exist, create one from .gitignore files",
			Default:     false,
		},
		flag.Int{
			Name:        "internal-port",
			Description: "Set internal_port for all services in the generated fly.toml",
			Default:     -1,
		},
		// Launch V2
		flag.Bool{
			Name:        "ui",
			Description: "Use the Launch V2 interface",
			Hidden:      true,
		},
	)

	return
}

func run(ctx context.Context) (err error) {

	if !flag.GetBool(ctx, "ui") {
		return legacy.Run(ctx)
	}

	io := iostreams.FromContext(ctx)

	if err := warnLegacyBehavior(ctx); err != nil {
		return err
	}

	// TODO: Metrics

	state, err := v2BuildPlan(ctx)
	if err != nil {
		return err
	}

	summary, err := state.PlanSummary(ctx)
	if err != nil {
		return err
	}

	fmt.Fprintf(
		io.Out,
		"We're about to launch your %s on Fly.io. Here's what you're getting:\n\n%s\n",
		familyToAppType(state.sourceInfo),
		summary,
	)

	confirm := false
	prompt := &survey.Confirm{
		Message: "Do you want to tweak these settings before proceeding?",
	}
	err = survey.AskOne(prompt, &confirm)
	if err != nil {
		// TODO(allison): This should probably not just return the error
		return err
	}

	if confirm {
		err = state.EditInWebUi(ctx)
		if err != nil {
			return err
		}
	}

	err = state.Launch(ctx)
	if err != nil {
		return err
	}

	return nil
}

type launchState struct {
	workingDir string
	configPath string
	plan       *launchPlan
	planSource *launchPlanSource
	env        map[string]string
	appConfig  *appconfig.Config
	sourceInfo *scanner.SourceInfo
	cache      map[string]interface{}
}

func cacheGrab[T any](cache map[string]interface{}, key string, cb func() (T, error)) (T, error) {
	if val, ok := cache[key]; ok {
		return val.(T), nil
	}
	val, err := cb()
	if err != nil {
		return val, err
	}
	cache[key] = val
	return val, nil
}

func (state *launchState) Org(ctx context.Context) (*api.Organization, error) {
	apiClient := client.FromContext(ctx).API()
	return cacheGrab(state.cache, "org,"+state.plan.OrgSlug, func() (*api.Organization, error) {
		return apiClient.GetOrganizationBySlug(ctx, state.plan.OrgSlug)
	})
}

func (state *launchState) Region(ctx context.Context) (api.Region, error) {

	apiClient := client.FromContext(ctx).API()
	regions, err := cacheGrab(state.cache, "regions", func() ([]api.Region, error) {
		regions, _, err := apiClient.PlatformRegions(ctx)
		if err != nil {
			return nil, err
		}
		return regions, nil
	})
	if err != nil {
		return api.Region{}, err
	}

	region, ok := lo.Find(regions, func(r api.Region) bool {
		return r.Code == state.plan.RegionCode
	})
	if !ok {
		return region, fmt.Errorf("region %state not found", state.plan.RegionCode)
	}
	return region, nil
}

// PlanSummary returns a human-readable summary of the launch plan.
// Used to confirm the plan before executing it.
func (state *launchState) PlanSummary(ctx context.Context) (string, error) {

	guest := state.plan.Guest()

	org, err := state.Org(ctx)
	if err != nil {
		return "", err
	}

	region, err := state.Region(ctx)
	if err != nil {
		return "", err
	}

	redisStr, err := state.plan.Redis.Describe(ctx)
	if err != nil {
		return "", err
	}

	rows := [][]string{
		{"Organization", org.Name, state.planSource.orgSource},
		{"Name", state.plan.AppName, state.planSource.appNameSource},
		{"Region", region.Name, state.planSource.regionSource},
		{"App Machines", guest.String(), state.planSource.guestSource},
		{"Postgres", state.plan.Postgres.Describe(), state.planSource.postgresSource},
		{"Redis", redisStr, state.planSource.redisSource},
	}

	colLengths := []int{0, 0, 0}
	for _, row := range rows {
		for i, col := range row {
			if len(col) > colLengths[i] {
				colLengths[i] = len(col)
			}
		}
	}

	ret := ""
	for _, row := range rows {

		label := row[0]
		value := row[1]
		source := row[2]

		labelSpaces := strings.Repeat(" ", colLengths[0]-len(label))
		valueSpaces := strings.Repeat(" ", colLengths[1]-len(value))

		ret += fmt.Sprintf("%state: %state%state %state(%state)\n", label, labelSpaces, value, valueSpaces, source)
	}
	return ret, nil
}

// familyToAppType returns a string that describes the app type based on the source info
// For example, "Dockerfile" apps would return "app" but a rails app would return "Rails app"
func familyToAppType(si *scanner.SourceInfo) string {
	if si == nil {
		return "app"
	}
	switch si.Family {
	case "Dockerfile":
		return "app"
	case "":
		return "app"
	}
	return fmt.Sprintf("%s app", si.Family)
}

// warnLegacyBehavior warns the user if they are using a legacy flag
func warnLegacyBehavior(ctx context.Context) error {
	// TODO(Allison): We probably want to support re-configuring an existing app, but
	// that is different from the launch-into behavior of reuse-app, which basically just deployed.
	if flag.IsSpecified(ctx, "reuse-app") {
		return errors.New("the --reuse-app flag is no longer supported. you are likely looking for 'fly deploy'")
	}
	return nil
}
