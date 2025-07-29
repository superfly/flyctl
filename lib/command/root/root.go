// Package root implements the root command.
package root

import (
	"context"
	"log"
	"os"
	"path/filepath"

	"github.com/kr/text"
	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/flyctl"
	"github.com/superfly/flyctl/lib/command"
	"github.com/superfly/flyctl/lib/command/agent"
	"github.com/superfly/flyctl/lib/command/apps"
	"github.com/superfly/flyctl/lib/command/auth"
	"github.com/superfly/flyctl/lib/command/certificates"
	"github.com/superfly/flyctl/lib/command/checks"
	"github.com/superfly/flyctl/lib/command/config"
	"github.com/superfly/flyctl/lib/command/console"
	"github.com/superfly/flyctl/lib/command/consul"
	"github.com/superfly/flyctl/lib/command/create"
	"github.com/superfly/flyctl/lib/command/curl"
	"github.com/superfly/flyctl/lib/command/dashboard"
	"github.com/superfly/flyctl/lib/command/deploy"
	"github.com/superfly/flyctl/lib/command/destroy"
	"github.com/superfly/flyctl/lib/command/dig"
	"github.com/superfly/flyctl/lib/command/dnsrecords"
	"github.com/superfly/flyctl/lib/command/docs"
	"github.com/superfly/flyctl/lib/command/doctor"
	"github.com/superfly/flyctl/lib/command/domains"
	"github.com/superfly/flyctl/lib/command/extensions"
	"github.com/superfly/flyctl/lib/command/history"
	"github.com/superfly/flyctl/lib/command/image"
	"github.com/superfly/flyctl/lib/command/incidents"
	"github.com/superfly/flyctl/lib/command/info"
	"github.com/superfly/flyctl/lib/command/ips"
	"github.com/superfly/flyctl/lib/command/jobs"
	"github.com/superfly/flyctl/lib/command/launch"
	"github.com/superfly/flyctl/lib/command/lfsc"
	"github.com/superfly/flyctl/lib/command/logs"
	"github.com/superfly/flyctl/lib/command/machine"
	"github.com/superfly/flyctl/lib/command/mcp"
	"github.com/superfly/flyctl/lib/command/metrics"
	"github.com/superfly/flyctl/lib/command/move"
	"github.com/superfly/flyctl/lib/command/mpg"
	"github.com/superfly/flyctl/lib/command/mysql"
	"github.com/superfly/flyctl/lib/command/open"
	"github.com/superfly/flyctl/lib/command/orgs"
	"github.com/superfly/flyctl/lib/command/ping"
	"github.com/superfly/flyctl/lib/command/platform"
	"github.com/superfly/flyctl/lib/command/postgres"
	"github.com/superfly/flyctl/lib/command/proxy"
	"github.com/superfly/flyctl/lib/command/redis"
	"github.com/superfly/flyctl/lib/command/regions"
	"github.com/superfly/flyctl/lib/command/registry"
	"github.com/superfly/flyctl/lib/command/releases"
	"github.com/superfly/flyctl/lib/command/resume"
	"github.com/superfly/flyctl/lib/command/scale"
	"github.com/superfly/flyctl/lib/command/secrets"
	"github.com/superfly/flyctl/lib/command/services"
	"github.com/superfly/flyctl/lib/command/settings"
	"github.com/superfly/flyctl/lib/command/ssh"
	"github.com/superfly/flyctl/lib/command/status"
	"github.com/superfly/flyctl/lib/command/storage"
	"github.com/superfly/flyctl/lib/command/suspend"
	"github.com/superfly/flyctl/lib/command/synthetics"
	"github.com/superfly/flyctl/lib/command/tokens"
	"github.com/superfly/flyctl/lib/command/version"
	"github.com/superfly/flyctl/lib/command/volumes"
	"github.com/superfly/flyctl/lib/command/wireguard"
	"github.com/superfly/flyctl/lib/flag/flagnames"
	"github.com/superfly/flyctl/lib/flyutil"
)

// New initializes and returns a reference to a new root command.
func New() *cobra.Command {
	const (
		long  = `This is flyctl, the Fly.io command line interface.`
		short = "The Fly.io command line interface"
	)

	exePath, err := os.Executable()
	var exe string
	if err != nil {
		log.Printf("WARN: failed to find executable, error=%q", err)
		exe = "fly"
	} else {
		exe = filepath.Base(exePath)
	}

	root := command.New(exe, short, long, run)
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
		group(deploy.New().Command, "deploy"),
		group(history.New(), "upkeep"),
		group(status.New(), "deploy"),
		group(logs.New(), "upkeep"),
		group(doctor.New(), "more_help"),
		group(dig.New(), "upkeep"),
		group(volumes.New(), "configuring"),
		group(lfsc.New(), "dbs_and_extensions"),
		agent.New(),
		group(image.New(), "configuring"),
		group(incidents.New(), "upkeep"),
		group(mysql.New(), "dbs_and_extensions"),
		group(ping.New(), "upkeep"),
		group(proxy.New(), "upkeep"),
		group(postgres.New(), "dbs_and_extensions"),
		group(mcp.New(), "upkeep"),
		group(mpg.New(), "dbs_and_extensions"),
		group(ips.New(), "configuring"),
		group(secrets.New(), "configuring"),
		group(ssh.New(), "upkeep"),
		group(ssh.NewSFTP(), "upkeep"),
		group(redis.New(), "dbs_and_extensions"),
		group(registry.New(), "upkeep"),
		group(checks.New(), "upkeep"),
		group(launch.New(), "deploy"),
		group(info.New(), "upkeep"),
		jobs.New(),
		group(services.New(), "upkeep"),
		group(config.New(), "configuring"),
		group(scale.New(), "configuring"),
		group(tokens.New(), "acl"),
		group(extensions.New(), "dbs_and_extensions"),
		group(consul.New(), "dbs_and_extensions"),
		group(certificates.New(), "configuring"),
		group(dashboard.New(), "upkeep"),
		group(wireguard.New(), "upkeep"),
		group(console.New(), "upkeep"),
		settings.New(),
		group(storage.New(), "dbs_and_extensions"),
		metrics.New(),
		synthetics.New(),
		curl.New(),       // TODO: deprecate
		domains.New(),    // TODO: deprecate
		open.New(),       // TODO: deprecate
		create.New(),     // TODO: deprecate
		destroy.New(),    // TODO: deprecate
		move.New(),       // TODO: deprecate
		suspend.New(),    // TODO: deprecate
		resume.New(),     // TODO: deprecate
		dnsrecords.New(), // TODO: deprecate

		regions.New(), // TODO: deprecate
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

	if !flyutil.ClientFromContext(ctx).Authenticated() {
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
