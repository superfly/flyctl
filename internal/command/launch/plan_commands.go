package launch

import (
	"context"
	"os"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/command/launch/plan"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/logger"
	"github.com/superfly/flyctl/iostreams"
)

func NewPlan() *cobra.Command {
	const desc = `[experimental] Granular subcommands for creating and configuring apps`

	cmd := command.New("plan", desc, desc, nil, command.RequireSession, command.RequireUiex, command.LoadAppConfigIfPresent)
	cmd.Args = cobra.NoArgs

	cmd.AddCommand(newPropose())
	cmd.AddCommand(newCreate())
	cmd.AddCommand(newPostgres())
	cmd.AddCommand(newRedis())
	cmd.AddCommand(newTigris())
	cmd.AddCommand(newGenerate())

	// Don't advertise this command yet
	cmd.Hidden = true

	return cmd
}

func newPropose() *cobra.Command {
	const desc = "[experimental] propose a plan based on scanning the source code or Dockerfile"
	cmd := command.New("propose", desc, desc, runPropose, command.LoadAppConfigIfPresent, command.RequireUiex)

	flag.Add(cmd,
		flag.Region(),
		flag.Org(),
		flag.String{
			Name:        "from",
			Description: "A github repo URL to use as a template for the new app",
		},
		flag.Bool{
			Name:        "no-create",
			Description: "Don't create an app",
			Default:     true,
			Hidden:      true,
		},
		flag.Bool{
			Name:        "manifest",
			Description: "Output the proposed manifest",
			Default:     true,
			Hidden:      true,
		},
		flag.String{
			Name:        "name",
			Description: `Name of the new app`,
		},
		flag.Bool{
			Name:   "force-name",
			Hidden: true,
		},
		flag.Bool{
			Name:        "copy-config",
			Description: "Use the configuration file if present without prompting",
			Default:     false,
		},
		flag.String{
			Name:        "manifest-path",
			Description: "Path to write the manifest to",
			Default:     "",
			Hidden:      true,
		},
		flag.Bool{
			Name:        "no-blank",
			Description: "Don't allow a \"blank\" app (nothing could be detected)",
			Default:     true,
		},
		flag.Compression(),
		flag.CompressionLevel(),
		flag.Int{
			Name:        "internal-port",
			Description: "Set internal_port for all services in the generated fly.toml",
			Default:     -1,
		},
	)

	return cmd
}

func newCreate() *cobra.Command {
	const desc = "[experimental] create application"
	cmd := command.New("create", desc, desc, runCreate, command.RequireUiex)
	cmd.Args = cobra.ExactArgs(1)

	flag.Add(cmd,
		flag.String{
			Name:        "from-manifest",
			Shorthand:   "p",
			Aliases:     []string{"manifest-path"},
			Description: "Path to read the manifest from",
			Default:     "",
			Hidden:      true,
		},
		flag.Int{
			Name:        "internal-port",
			Description: "Set internal_port for all services in the generated fly.toml",
			Default:     -1,
		},
		flag.Compression(),
		flag.CompressionLevel(),
	)

	return cmd
}

func newPostgres() *cobra.Command {
	const desc = "[experimental] create postgres database"
	cmd := command.New("postgres", desc, desc, runPostgres, command.RequireUiex)
	cmd.Args = cobra.ExactArgs(1)

	flag.Add(cmd,
		flag.String{
			Name:        "from-manifest",
			Shorthand:   "p",
			Aliases:     []string{"manifest-path"},
			Description: "Path to read the manifest from",
			Default:     "",
			Hidden:      true,
		},
	)

	return cmd
}

func newRedis() *cobra.Command {
	const desc = "[experimental] create redis database"
	cmd := command.New("redis", desc, desc, runRedis, command.RequireUiex)
	cmd.Args = cobra.ExactArgs(1)

	flag.Add(cmd,
		flag.String{
			Name:        "from-manifest",
			Shorthand:   "p",
			Aliases:     []string{"manifest-path"},
			Description: "Path to read the manifest from",
			Default:     "",
			Hidden:      true,
		},
	)

	return cmd
}

func newTigris() *cobra.Command {
	const desc = "[experimental] create tigris database"
	cmd := command.New("tigris", desc, desc, runTigris, command.RequireUiex)
	cmd.Args = cobra.ExactArgs(1)

	flag.Add(cmd,
		flag.String{
			Name:        "from-manifest",
			Shorthand:   "p",
			Aliases:     []string{"manifest-path"},
			Description: "Path to read the manifest from",
			Default:     "",
			Hidden:      true,
		},
	)

	return cmd
}

func newGenerate() *cobra.Command {
	const desc = "[experimental] generate Dockerfile and other configuration files based on the plan"
	cmd := command.New("generate", desc, desc, runGenerate, command.RequireUiex)
	cmd.Args = cobra.ExactArgs(1)

	flag.Add(cmd,
		// Experimentally re-enabling these
		flag.App(),
		flag.Region(),
		flag.Org(),
		flag.AppConfig(),
		// Experimentally re-enabling these
		flag.Bool{
			Name:        "no-deploy",
			Description: "Don't deploy the app",
			Default:     true,
			Hidden:      true,
		},
		flag.String{
			Name:        "from-manifest",
			Shorthand:   "p",
			Aliases:     []string{"manifest-path"},
			Description: "Path to read the manifest from",
			Default:     "",
			Hidden:      true,
		},
		flag.Compression(),
		flag.CompressionLevel(),
		flag.Int{
			Name:        "internal-port",
			Description: "Set internal_port for all services in the generated fly.toml",
			Default:     -1,
		},
	)

	return cmd
}

func RunPlan(ctx context.Context, step string) error {
	ctx = context.WithValue(ctx, plan.PlanStepKey, step)
	return run(ctx)
}

func runPropose(ctx context.Context) error {
	if flag.GetString(ctx, "manifest-path") == "" {
		ctx = logger.NewContext(context.Background(), logger.New(os.Stderr, logger.FromContext(ctx).Level(), iostreams.IsTerminalWriter(os.Stdout)))
	}

	RunPlan(ctx, "propose")
	return nil
}

func runCreate(ctx context.Context) error {
	flag.SetString(ctx, "from-manifest", flag.FirstArg(ctx))
	RunPlan(ctx, "create")
	return nil
}

func runPostgres(ctx context.Context) error {
	flag.SetString(ctx, "from-manifest", flag.FirstArg(ctx))
	RunPlan(ctx, "postgres")
	return nil
}

func runRedis(ctx context.Context) error {
	flag.SetString(ctx, "from-manifest", flag.FirstArg(ctx))
	RunPlan(ctx, "redis")
	return nil
}

func runTigris(ctx context.Context) error {
	flag.SetString(ctx, "from-manifest", flag.FirstArg(ctx))
	RunPlan(ctx, "tigris")
	return nil
}

func runGenerate(ctx context.Context) error {
	flag.SetString(ctx, "from-manifest", flag.FirstArg(ctx))
	return RunPlan(ctx, "generate")
}
