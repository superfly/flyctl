package mpg

import (
	"cmp"
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command"
	cmdv2 "github.com/superfly/flyctl/internal/command/mpg/v2"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/prompt"
	"github.com/superfly/flyctl/iostreams"
)

func newCreate() *cobra.Command {
	const (
		short = "Create a new Managed Postgres cluster"
		long  = short + "\n"
	)

	cmd := command.New("create", short, long, runCreate,
		command.RequireSession,
		requireMacaroonToken,
	)

	flag.Add(
		cmd,
		flag.Region(),
		flag.Org(),
		flag.String{
			Name:        "name",
			Shorthand:   "n",
			Description: "The name of your Postgres cluster",
		},
		flag.Bool{
			Name:        "v2",
			Description: "Deprecated: Managed Postgres clusters are always created on v2",
			Default:     true,
			Hidden:      true,
		},
		flag.String{
			Name:        "plan",
			Description: "The plan to use for the Postgres cluster: Basic, Starter, Launch, Scale, Performance",
		},
		flag.Int{
			Name:        "volume-size",
			Description: "Initial volume size in GB",
			Default:     10,
		},
		flag.Bool{
			Name:        "enable-postgis-support",
			Description: "Enable PostGIS for the Postgres cluster",
			Default:     false,
		},
		flag.Int{
			Name:        "pg-major-version",
			Description: "The major version of Postgres to use for the Postgres cluster. Supported versions are 16 and 17.",
			Default:     16,
		},
	)

	return cmd
}

// warnIfV2FlagUsed tells the user that --v2 no longer does anything. The flag is
// kept so scripts passing it keep working, since cobra rejects unknown flags.
func warnIfV2FlagUsed(ctx context.Context) {
	if !flag.IsSpecified(ctx, "v2") {
		return
	}

	io := iostreams.FromContext(ctx)
	fmt.Fprintln(io.ErrOut, io.ColorScheme().Yellow("The '--v2' flag is deprecated and no longer has any effect. Managed Postgres clusters are always created on v2."))
}

func runCreate(ctx context.Context) error {
	var (
		io      = iostreams.FromContext(ctx)
		appName = flag.GetString(ctx, "name")
		err     error
	)

	warnIfV2FlagUsed(ctx)

	if appName == "" {
		// If no name is provided, try to get the app name from context
		if appName = appconfig.NameFromContext(ctx); appName != "" {
			// If we have an app name, use it to create a default database name
			appName = appName + "-db"
		} else {
			// If no app name is available, prompt for a name
			appName, err = prompt.SelectAppNameWithMsg(ctx, "Choose a database name:")
			if err != nil {
				return err
			}
		}
	}

	org, err := prompt.Org(ctx)
	if err != nil {
		return err
	}

	pgMajorVersion := flag.GetInt(ctx, "pg-major-version")
	if pgMajorVersion != 16 && pgMajorVersion != 17 {
		return fmt.Errorf("invalid Postgres major version: %d. Supported versions are 16 and 17", pgMajorVersion)
	}

	// For ui-ex request we need the real org slug
	slug := org.Slug
	if org.Slug == "personal" {
		slug = org.RawSlug
	}

	// Plan selection and validation
	plans := orgPlans(ctx, slug)
	plan, ok := planKey(plans, flag.GetString(ctx, "plan"))
	if !ok {
		if !io.IsInteractive() && flag.IsSpecified(ctx, "plan") {
			return fmt.Errorf("unknown plan %q; valid plans are %s", flag.GetString(ctx, "plan"), planNames(plans))
		}

		if io.IsInteractive() {
			planOptions, planKeys, defaultOption := planPrompt(plans)
			var selectedIndex int
			if err := prompt.Select(ctx, &selectedIndex, "Select a plan for your Managed Postgres cluster", defaultOption, planOptions...); err != nil {
				return err
			}
			plan = planKeys[selectedIndex]
		} else {
			plan = defaultPlan(plans)
		}
	}

	params := &cmdv2.CreateClusterParams{
		Name:           appName,
		OrgSlug:        slug,
		Plan:           plan,
		PGMajorVersion: pgMajorVersion,
		StorageInGb:    flag.GetInt(ctx, "volume-size"),
		PostGISEnabled: flag.GetBool(ctx, "enable-postgis-support"),
	}

	planDetails := plans[plan]

	return cmdv2.RunCreate(ctx, org.RawSlug, params, &cmdv2.CreatePlanDisplay{
		Name:       planDetails.Name,
		CPU:        planDetails.CPU,
		Memory:     planDetails.Memory,
		PricePerMo: planDetails.PricePerMo,
	})
}

// planKey returns the key in plans for a plan name given in any case. The keys
// are what the API expects, so "performance" has to find "Performance" (#4612)
// rather than be lowercased.
func planKey(plans map[string]PlanDetails, name string) (string, bool) {
	name = strings.TrimSpace(name)
	for key := range plans {
		if strings.EqualFold(key, name) {
			return key, true
		}
	}

	return "", false
}

// planKeysByPrice lists the keys of plans cheapest first.
func planKeysByPrice(plans map[string]PlanDetails) []string {
	return slices.SortedFunc(maps.Keys(plans), func(a, b string) int {
		return cmp.Compare(plans[a].PricePerMo, plans[b].PricePerMo)
	})
}

// planNames lists the plans cheapest first, as --plan's help does.
func planNames(plans map[string]PlanDetails) string {
	keys := planKeysByPrice(plans)

	names := make([]string, len(keys))
	for i, key := range keys {
		names[i] = plans[key].Name
	}

	return strings.Join(names, ", ")
}

// defaultPlan is the plan created when none is chosen: Basic, so a cheaper
// plan such as dev-256 doesn't become the default, or the cheapest plan if
// the org isn't offered Basic.
func defaultPlan(plans map[string]PlanDetails) string {
	if _, ok := plans["basic"]; ok {
		return "basic"
	}

	return planKeysByPrice(plans)[0]
}

// planPrompt returns the plan prompt's options cheapest first, the key behind
// each, and the option to preselect, which is the default plan's.
func planPrompt(plans map[string]PlanDetails) (options, keys []string, def string) {
	keys = planKeysByPrice(plans)
	for _, key := range keys {
		p := plans[key]
		options = append(options, fmt.Sprintf("%s: %s, %s RAM, $%d/mo", p.Name, p.CPU, p.Memory, p.PricePerMo))
	}

	return options, keys, options[slices.Index(keys, defaultPlan(plans))]
}
