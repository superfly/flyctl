// Package root implements the root command.
package root

import (
	"context"

	"github.com/kr/text"
	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/flyctl"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/command/agent"
	"github.com/superfly/flyctl/internal/command/apps"
	"github.com/superfly/flyctl/internal/command/auth"
	"github.com/superfly/flyctl/internal/command/autoscale"
	"github.com/superfly/flyctl/internal/command/certificates"
	"github.com/superfly/flyctl/internal/command/checks"
	"github.com/superfly/flyctl/internal/command/config"
	"github.com/superfly/flyctl/internal/command/console"
	"github.com/superfly/flyctl/internal/command/consul"
	"github.com/superfly/flyctl/internal/command/create"
	"github.com/superfly/flyctl/internal/command/curl"
	"github.com/superfly/flyctl/internal/command/dashboard"
	"github.com/superfly/flyctl/internal/command/deploy"
	"github.com/superfly/flyctl/internal/command/destroy"
	"github.com/superfly/flyctl/internal/command/dig"
	"github.com/superfly/flyctl/internal/command/dnsrecords"
	"github.com/superfly/flyctl/internal/command/docs"
	"github.com/superfly/flyctl/internal/command/doctor"
	"github.com/superfly/flyctl/internal/command/domains"
	"github.com/superfly/flyctl/internal/command/extensions"
	"github.com/superfly/flyctl/internal/command/history"
	"github.com/superfly/flyctl/internal/command/image"
	"github.com/superfly/flyctl/internal/command/info"
	"github.com/superfly/flyctl/internal/command/ips"
	"github.com/superfly/flyctl/internal/command/jobs"
	"github.com/superfly/flyctl/internal/command/launch"
	"github.com/superfly/flyctl/internal/command/lfsc"
	"github.com/superfly/flyctl/internal/command/logs"
	"github.com/superfly/flyctl/internal/command/machine"
	"github.com/superfly/flyctl/internal/command/migrate_to_v2"
	"github.com/superfly/flyctl/internal/command/monitor"
	"github.com/superfly/flyctl/internal/command/move"
	"github.com/superfly/flyctl/internal/command/mysql"
	"github.com/superfly/flyctl/internal/command/open"
	"github.com/superfly/flyctl/internal/command/orgs"
	"github.com/superfly/flyctl/internal/command/ping"
	"github.com/superfly/flyctl/internal/command/platform"
	"github.com/superfly/flyctl/internal/command/postgres"
	"github.com/superfly/flyctl/internal/command/proxy"
	"github.com/superfly/flyctl/internal/command/redis"
	"github.com/superfly/flyctl/internal/command/regions"
	"github.com/superfly/flyctl/internal/command/releases"
	"github.com/superfly/flyctl/internal/command/resume"
	"github.com/superfly/flyctl/internal/command/scale"
	"github.com/superfly/flyctl/internal/command/secrets"
	"github.com/superfly/flyctl/internal/command/services"
	"github.com/superfly/flyctl/internal/command/settings"
	"github.com/superfly/flyctl/internal/command/ssh"
	"github.com/superfly/flyctl/internal/command/status"
	"github.com/superfly/flyctl/internal/command/suspend"
	"github.com/superfly/flyctl/internal/command/tokens"
	"github.com/superfly/flyctl/internal/command/turboku"
	"github.com/superfly/flyctl/internal/command/version"
	"github.com/superfly/flyctl/internal/command/vm"
	"github.com/superfly/flyctl/internal/command/volumes"
	"github.com/superfly/flyctl/internal/command/wireguard"
	"github.com/superfly/flyctl/internal/flag/flagnames"
)

// New initializes and returns a reference to a new root command.
func New() *cobra.Command {
	const (
		long  = `This is flyctl, the Fly.io command line interface.`
		short = "The Fly.io command line interface"
	)

	root := command.New("flyctl", short, long, run)
	root.PersistentPreRun = func(cmd *cobra.Command, args []string) {
		cmd.SilenceUsage = true
		cmd.SilenceErrors = true
	}

	fs := root.PersistentFlags()
	_ = fs.StringP(flagnames.AccessToken, "t", "", "Fly API Access Token")
	_ = fs.BoolP(flagnames.Verbose, "", false, "Verbose output")
	_ = fs.BoolP(flagnames.Debug, "", false, "Print additional logs and traces")

	flyctl.InitConfig()

	root.AddCommand(
		group(apps.New(), "deploy"),
		group(machine.New(), "deploy"),
		version.New(),
		group(orgs.New(), "acl"),
		group(auth.New(), "acl"),
		group(platform.New(), "more_help"),
		group(docs.New(), "more_help"),
		group(releases.New(), "upkeep"),
		group(deploy.New(), "deploy"),
		group(history.New(), "upkeep"),
		group(status.New(), "deploy"),
		group(logs.New(), "upkeep"),
		group(doctor.New(), "more_help"),
		group(dig.New(), "upkeep"),
		group(volumes.New(), "configuring"),
		group(lfsc.New(), "dbs_and_extensions"),
		agent.New(),
		group(image.New(), "configuring"),
		group(ping.New(), "upkeep"),
		group(proxy.New(), "upkeep"),
		group(monitor.New(), "apps_v1"),
		group(postgres.New(), "dbs_and_extensions"),
		group(ips.New(), "configuring"),
		group(secrets.New(), "configuring"),
		group(ssh.New(), "upkeep"),
		group(ssh.NewSFTP(), "upkeep"),
		group(redis.New(), "dbs_and_extensions"),
		group(vm.New(), "apps_v1"),
		group(checks.New(), "upkeep"),
		group(launch.New(), "deploy"),
		group(info.New(), "upkeep"),
		jobs.New(),
		turboku.New(),
		group(services.New(), "upkeep"),
		group(config.New(), "configuring"),
		group(scale.New(), "configuring"),
		group(migrate_to_v2.New(), "apps_v1"),
		group(tokens.New(), "acl"),
		group(extensions.New(), "dbs_and_extensions"),
		group(consul.New(), "dbs_and_extensions"),
		group(regions.New(), "apps_v1"),
		group(certificates.New(), "configuring"),
		group(dashboard.New(), "upkeep"),
		group(wireguard.New(), "upkeep"),
		group(autoscale.New(), "apps_v1"),
		group(console.New(), "upkeep"),
		settings.New(),
		group(mysql.New(), "dbs_and_extensions"),
		curl.New(),       // TODO: deprecate
		domains.New(),    // TODO: deprecate
		open.New(),       // TODO: deprecate
		create.New(),     // TODO: deprecate
		destroy.New(),    // TODO: deprecate
		move.New(),       // TODO: deprecate
		suspend.New(),    // TODO: deprecate
		resume.New(),     // TODO: deprecate
		dnsrecords.New(), // TODO: deprecate
	)

	// if os.Getenv("DEV") != "" {
	// 	newCommands = append(newCommands, services.New())
	// }

	// root.SetHelpCommand(help.New(root))
	// root.RunE = help.NewRootHelp().RunE

	root.AddGroup(&cobra.Group{
		ID:    "deploy",
		Title: "Deploying apps & machines",
	})
	root.AddGroup(&cobra.Group{
		ID:    "configuring",
		Title: "Configuration & scaling",
	})
	root.AddGroup(&cobra.Group{
		ID:    "upkeep",
		Title: "Monitoring & managing things",
	})
	root.AddGroup(&cobra.Group{
		ID:    "dbs_and_extensions",
		Title: "Databases & extensions",
	})
	root.AddGroup(&cobra.Group{
		ID:    "acl",
		Title: "Access control",
	})
	root.AddGroup(&cobra.Group{
		ID:    "more_help",
		Title: "Help & troubleshooting",
	})
	root.AddGroup(&cobra.Group{
		ID:    "apps_v1",
		Title: "Apps v1 (deprecated)",
	})

	return root
}

func run(ctx context.Context) error {
	cmd := command.FromContext(ctx)

	cmd.Println(cmd.Long)
	cmd.Println()
	cmd.Println("Usage:")
	cmd.Printf("  %s\n", cmd.UseLine())
	cmd.Printf("  %s\n", "flyctl [command]")
	cmd.Println()

	if !client.FromContext(ctx).Authenticated() {
		msg := `It doesn't look like you're logged in. Try "fly auth signup" to create an account, or "fly auth login" to log in to an existing account.`
		cmd.Println(text.Wrap(msg, 80))
		cmd.Println()
	}

	cmd.Println("Here's a few commands to get you started:")

	importantCommands := [][]string{
		{"launch"},
		{"status"},
		{"deploy"},
		{"logs"},
		{"apps"},
		{"machines"},
	}

	for _, path := range importantCommands {
		c, _, err := cmd.Traverse(path)
		if err != nil {
			panic(err)
		}
		cmd.Printf("  %s %s\n", tablewriter.PadRight(c.CommandPath(), " ", 16), c.Short)
	}

	cmd.Println()

	cmd.Println("If you need help along the way:")
	cmd.Println("  Use `fly docs` to open the Fly.io documentation, or visit https://fly.io/docs.")
	cmd.Println("  Use `fly <command> --help` for more information about a command.")
	cmd.Println("  Visit https://community.fly.io to get help from the Fly.io community.")

	cmd.Println()
	cmd.Println("For a full list of commands, run `fly help`.")

	return nil
}

func group(cmd *cobra.Command, id string) *cobra.Command {
	cmd.GroupID = id
	return cmd
}
