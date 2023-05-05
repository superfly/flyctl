// Package root implements the root command.
package root

import (
	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/flyctl"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/command/agent"
	"github.com/superfly/flyctl/internal/command/apps"
	"github.com/superfly/flyctl/internal/command/auth"
	"github.com/superfly/flyctl/internal/command/autoscale"
	"github.com/superfly/flyctl/internal/command/certificates"
	"github.com/superfly/flyctl/internal/command/checks"
	"github.com/superfly/flyctl/internal/command/config"
	"github.com/superfly/flyctl/internal/command/consul"
	"github.com/superfly/flyctl/internal/command/create"
	"github.com/superfly/flyctl/internal/command/curl"
	"github.com/superfly/flyctl/internal/command/dashboard"
	"github.com/superfly/flyctl/internal/command/deploy"
	"github.com/superfly/flyctl/internal/command/destroy"
	"github.com/superfly/flyctl/internal/command/dig"
	dnsrecords "github.com/superfly/flyctl/internal/command/dns-records"
	"github.com/superfly/flyctl/internal/command/docs"
	"github.com/superfly/flyctl/internal/command/doctor"
	"github.com/superfly/flyctl/internal/command/domains"
	"github.com/superfly/flyctl/internal/command/extensions"
	"github.com/superfly/flyctl/internal/command/help"
	"github.com/superfly/flyctl/internal/command/history"
	"github.com/superfly/flyctl/internal/command/image"
	"github.com/superfly/flyctl/internal/command/info"
	"github.com/superfly/flyctl/internal/command/ips"
	"github.com/superfly/flyctl/internal/command/jobs"
	"github.com/superfly/flyctl/internal/command/launch"
	"github.com/superfly/flyctl/internal/command/logs"
	"github.com/superfly/flyctl/internal/command/machine"
	"github.com/superfly/flyctl/internal/command/migrate_to_v2"
	"github.com/superfly/flyctl/internal/command/monitor"
	"github.com/superfly/flyctl/internal/command/move"
	"github.com/superfly/flyctl/internal/command/open"
	"github.com/superfly/flyctl/internal/command/orgs"
	"github.com/superfly/flyctl/internal/command/ping"
	"github.com/superfly/flyctl/internal/command/platform"
	"github.com/superfly/flyctl/internal/command/postgres"
	"github.com/superfly/flyctl/internal/command/proxy"
	"github.com/superfly/flyctl/internal/command/redis"
	"github.com/superfly/flyctl/internal/command/regions"
	"github.com/superfly/flyctl/internal/command/releases"
	"github.com/superfly/flyctl/internal/command/restart"
	"github.com/superfly/flyctl/internal/command/resume"
	"github.com/superfly/flyctl/internal/command/scale"
	"github.com/superfly/flyctl/internal/command/secrets"
	"github.com/superfly/flyctl/internal/command/services"
	"github.com/superfly/flyctl/internal/command/ssh"
	"github.com/superfly/flyctl/internal/command/status"
	"github.com/superfly/flyctl/internal/command/suspend"
	"github.com/superfly/flyctl/internal/command/tokens"
	"github.com/superfly/flyctl/internal/command/turboku"
	"github.com/superfly/flyctl/internal/command/version"
	"github.com/superfly/flyctl/internal/command/vm"
	"github.com/superfly/flyctl/internal/command/volumes"
	"github.com/superfly/flyctl/internal/command/wireguard"
	"github.com/superfly/flyctl/internal/flag"
)

// New initializes and returns a reference to a new root command.
func New() *cobra.Command {
	const (
		long = `flyctl is a command line interface to the Fly.io platform.

It allows users to manage authentication, application launch,
deployment, network configuration, logging and more with just the
one command.

* Launch an app with the launch command
* Deploy an app with the deploy command
* View a deployed web application with the open command
* Check the status of an application with the status command

To read more, use the docs command to view Fly's help on the web.
		`
		short = "The Fly CLI"
	)

	root := command.New("flyctl", short, long, nil)
	root.PersistentPreRun = func(cmd *cobra.Command, args []string) {
		cmd.SilenceUsage = true
		cmd.SilenceErrors = true
	}

	fs := root.PersistentFlags()
	_ = fs.StringP(flag.AccessTokenName, "t", "", "Fly API Access Token")
	_ = fs.BoolP(flag.VerboseName, "", false, "Verbose output")

	flyctl.InitConfig()

	// what follows is a hack in order to achieve compatibility with what exists
	// already. the commented out code above, is what should remain after the
	// migration is complete.

	// newCommands is the set of commands which work with the new way
	root.AddCommand(
		version.New(),
		apps.New(),
		create.New(),  // TODO: deprecate
		destroy.New(), // TODO: deprecate
		move.New(),    // TODO: deprecate
		suspend.New(), // TODO: deprecate
		resume.New(),  // TODO: deprecate
		restart.New(), // TODO: deprecate
		orgs.New(),
		auth.New(),
		open.New(), // TODO: deprecate
		curl.New(),
		platform.New(),
		docs.New(),
		releases.New(),
		deploy.New(),
		history.New(),
		status.New(),
		logs.New(),
		doctor.New(),
		dig.New(),
		volumes.New(),
		agent.New(),
		image.New(),
		ping.New(),
		proxy.New(),
		machine.New(),
		monitor.New(),
		postgres.New(),
		ips.New(),
		secrets.New(),
		ssh.New(),
		ssh.NewSFTP(),
		redis.New(),
		vm.New(),
		checks.New(),
		launch.New(),
		info.New(),
		jobs.New(),
		turboku.New(),
		services.New(),
		config.New(),
		scale.New(),
		migrate_to_v2.New(),
		tokens.New(),
		extensions.New(),
		consul.New(),
		regions.New(),
		dnsrecords.New(),
		certificates.New(),
		dashboard.New(),
		wireguard.New(),
		autoscale.New(),
		domains.New(),
	)

	// if os.Getenv("DEV") != "" {
	// 	newCommands = append(newCommands, services.New())
	// }

	root.SetHelpCommand(help.New(root))
	root.RunE = help.NewRootHelp().RunE
	return root
}
