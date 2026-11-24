package mpg

import (
	"cmp"
	"context"
	"fmt"
	"maps"
	"slices"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command"
	cmdv2 "github.com/superfly/flyctl/internal/command/mpg/v2"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/mpgutil"
	"github.com/superfly/flyctl/internal/prompt"
	"github.com/superfly/flyctl/iostreams"
)

func newCreate() *cobra.Command {
	const (
		short = "Create a Managed Postgres cluster"
		long  = `Create a Managed Postgres cluster and wait until it's ready, then print
its connection string, including the password. The plan sets the
cluster's CPU, memory and monthly price.

The name, organization, region and plan come from --name, --org (or
FLY_ORG), --region and --plan, or you're asked for them in a terminal. If
your only organization is your personal one, it's used without asking.
Without a terminal, --name and --region are required, so is --org unless
FLY_ORG is set or you only have a personal organization, and the plan
defaults to Basic.

You can stop waiting with Ctrl+C; the cluster keeps provisioning. Then use
` + "`fly mpg status`" + ` to check on it and ` + "`fly mpg attach`" + ` to connect an app.`
	)

	cmd := command.New("create", short, long, runCreate,
		requireMutationSession,
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
			Description: "The plan to use for the Postgres cluster: " + planNames(),
		},
		flag.Int{
			Name:        "volume-size",
			Description: "Initial volume size in GB. Defaults to the plan's size.",
		},
		flag.Bool{
			Name:        "enable-postgis-support",
			Description: "Enable PostGIS for the Postgres cluster",
			Default:     false,
		},
		flag.Int{
			Name:        "pg-major-version",
			Description: "The major version of Postgres to use for the Postgres cluster. Supported versions are " + mpgutil.SupportedPGMajorVersionsText() + ".",
			Default:     mpgutil.DefaultPGMajorVersion,
		},
	)

	addStdoutJSONFlag(cmd, "JSON output (id, name, organization, region, plan, status; progress on stderr)",
		`  out=$(fly mpg create --name my-db --org my-org --region iad --plan basic --json) || { echo "$out" >&2; exit 1; }
  cluster_id=$(printf '%s' "$out" | jq -er '.id | select(. != "")') || exit 1`)

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
			if config.FromContext(ctx).JSONOutput {
				return prompt.NonInteractiveError("--name must be specified when using --json")
			}
			// If no app name is available, prompt for a name
			appName, err = prompt.SelectAppNameWithMsg(ctx, "Choose a database name:")
			if err != nil {
				return err
			}
		}
	}

	if config.FromContext(ctx).JSONOutput {
		streams := *io
		streams.SetNeverPrompt(true)
		ctx = iostreams.NewContext(ctx, &streams)
	}
	org, err := prompt.Org(ctx)
	if err != nil {
		if config.FromContext(ctx).JSONOutput && prompt.IsNonInteractive(err) {
			return prompt.NonInteractiveError("--org must be specified when using --json")
		}
		return err
	}

	pgMajorVersion := flag.GetInt(ctx, "pg-major-version")
	if err := mpgutil.ValidatePGMajorVersion(pgMajorVersion); err != nil {
		return err
	}

	// Plan selection and validation
	plan, ok := planKey(flag.GetString(ctx, "plan"))
	if !ok {
		if (!io.IsInteractive() || config.FromContext(ctx).JSONOutput) && flag.IsSpecified(ctx, "plan") {
			return fmt.Errorf("unknown plan %q; valid plans are %s", flag.GetString(ctx, "plan"), planNames())
		}

		if io.IsInteractive() && !config.FromContext(ctx).JSONOutput {
			// Prepare a sortable slice of plans
			type planEntry struct {
				Key   string
				Value PlanDetails
			}
			var planEntries []planEntry
			for k, v := range MPGPlans {
				planEntries = append(planEntries, planEntry{Key: k, Value: v})
			}
			// Sort by price
			sort.Slice(planEntries, func(i, j int) bool {
				return planEntries[i].Value.PricePerMo < planEntries[j].Value.PricePerMo
			})
			// Build options and keys in sorted order
			var planOptions []string
			var planKeys []string
			for _, entry := range planEntries {
				planOptions = append(planOptions, fmt.Sprintf("%s: %s, %s RAM, $%d/mo", entry.Value.Name, entry.Value.CPU, entry.Value.Memory, entry.Value.PricePerMo))
				planKeys = append(planKeys, entry.Key)
			}
			// Developer is listed first, as the cheapest, but Basic stays the
			// default, as it is without a terminal.
			defaultOption := planOptions[slices.Index(planKeys, "basic")]
			var selectedIndex int
			if err := prompt.Select(ctx, &selectedIndex, "Select a plan for your Managed Postgres cluster", defaultOption, planOptions...); err != nil {
				return err
			}
			plan = planKeys[selectedIndex]
		} else {
			plan = "basic" // Default to basic if not interactive
		}
	}

	// For ui-ex request we need the real org slug
	slug := org.Slug
	if org.Slug == "personal" {
		slug = org.RawSlug
	}

	params := &cmdv2.CreateClusterParams{
		Name:           appName,
		OrgSlug:        slug,
		OrgID:          org.InternalNumericID,
		Plan:           plan,
		PGMajorVersion: pgMajorVersion,
		PostGISEnabled: flag.GetBool(ctx, "enable-postgis-support"),
	}

	// Without --volume-size no size is sent, so the API gives the cluster
	// its plan's own default disk size.
	if flag.IsSpecified(ctx, "volume-size") {
		params.StorageInGb = flag.GetInt(ctx, "volume-size")
	}

	planDetails := MPGPlans[plan]

	return cmdv2.RunCreate(ctx, org.RawSlug, params, &cmdv2.CreatePlanDisplay{
		Name:       planDetails.Name,
		CPU:        planDetails.CPU,
		Memory:     planDetails.Memory,
		PricePerMo: planDetails.PricePerMo,
	})
}

// planKey returns the MPGPlans key for a plan given by its key or its name
// (Developer for dev-256), in any case. The keys are what the API expects, so
// "performance" has to find "Performance" (#4612) rather than be lowercased.
func planKey(name string) (string, bool) {
	name = strings.TrimSpace(name)
	for key, details := range MPGPlans {
		if strings.EqualFold(key, name) || strings.EqualFold(details.Name, name) {
			return key, true
		}
	}

	return "", false
}

// planNames lists the plans, cheapest first, by the names --plan takes.
func planNames() string {
	keys := slices.SortedFunc(maps.Keys(MPGPlans), func(a, b string) int {
		return cmp.Compare(MPGPlans[a].PricePerMo, MPGPlans[b].PricePerMo)
	})

	names := make([]string, 0, len(keys))
	for _, key := range keys {
		// A plan whose name isn't its key, like Developer (dev-256), is
		// listed by its key, capitalized like the others: Dev-256.
		name := MPGPlans[key].Name
		if !strings.EqualFold(name, key) {
			name = strings.ToUpper(key[:1]) + key[1:]
		}
		names = append(names, name)
	}

	return strings.Join(names, ", ")
}
