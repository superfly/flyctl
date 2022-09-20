// Package root implements the root command.
package root

import (
	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/cmd"
	"github.com/superfly/flyctl/flyctl"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/command/agent"
	"github.com/superfly/flyctl/internal/command/apps"
	"github.com/superfly/flyctl/internal/command/auth"
	"github.com/superfly/flyctl/internal/command/checks"
	"github.com/superfly/flyctl/internal/command/create"
	"github.com/superfly/flyctl/internal/command/curl"
	"github.com/superfly/flyctl/internal/command/deploy"
	"github.com/superfly/flyctl/internal/command/destroy"
	"github.com/superfly/flyctl/internal/command/dig"
	"github.com/superfly/flyctl/internal/command/docs"
	"github.com/superfly/flyctl/internal/command/doctor"
	"github.com/superfly/flyctl/internal/command/history"
	"github.com/superfly/flyctl/internal/command/image"
	"github.com/superfly/flyctl/internal/command/ips"
	"github.com/superfly/flyctl/internal/command/logs"
	"github.com/superfly/flyctl/internal/command/machine"
	"github.com/superfly/flyctl/internal/command/monitor"
	"github.com/superfly/flyctl/internal/command/move"
	"github.com/superfly/flyctl/internal/command/open"
	"github.com/superfly/flyctl/internal/command/orgs"
	"github.com/superfly/flyctl/internal/command/ping"
	"github.com/superfly/flyctl/internal/command/platform"
	"github.com/superfly/flyctl/internal/command/postgres"
	"github.com/superfly/flyctl/internal/command/proxy"
	"github.com/superfly/flyctl/internal/command/redis"
	"github.com/superfly/flyctl/internal/command/releases"
	"github.com/superfly/flyctl/internal/command/restart"
	"github.com/superfly/flyctl/internal/command/resume"
	"github.com/superfly/flyctl/internal/command/secrets"
	"github.com/superfly/flyctl/internal/command/ssh"
	"github.com/superfly/flyctl/internal/command/status"
	"github.com/superfly/flyctl/internal/command/suspend"
	"github.com/superfly/flyctl/internal/command/version"
	"github.com/superfly/flyctl/internal/command/vm"
	"github.com/superfly/flyctl/internal/command/volumes"
)

// New initializes and returns a reference to a new root command.
func New() *cobra.Command {
	/*
			const (
				long = `flyctl is a command line interface to the Fly.io platform.

		It allows users to manage authentication, application launch,
		deployment, network configuration, logging and more with just the
		one command.

		Launch an app with the launch command
		Deploy an app with the deploy command
		View a deployed web application with the open command
		Check the status of an application with the status command

		To read more, use the docs command to view Fly's help on the web.
		`
				short = "The Fly CLI"
				usage = "flyctl"
			)

			root := command.New(usage, short, long, nil)
			root.SilenceUsage = true
			root.SilenceErrors = true

			fs := root.PersistentFlags()

			_ = fs.StringP(flag.AccessTokenName, "t", "", "Fly API Access Token")
			_ = fs.BoolP(flag.JSONOutputName, "j", false, "JSON output")
			_ = fs.BoolP(flag.VerboseName, "v", false, "Verbose output")

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
				builds.New(),
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
			)

			if os.Getenv("DEV") != "" {
				root.AddCommand(services.New())
			}

			return root
	*/

	flyctl.InitConfig()

	// what follows is a hack in order to achieve compatibility with what exists
	// already. the commented out code above, is what should remain after the
	// migration is complete.

	// newCommands is the set of commands which work with the new way
	newCommands := []*cobra.Command{
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
		redis.New(),
		vm.New(),
		checks.New(),
	}

	// if os.Getenv("DEV") != "" {
	// 	newCommands = append(newCommands, services.New())
	// }

	// newCommandNames is the set of the names of the above commands
	newCommandNames := make(map[string]struct{}, len(newCommands))
	for _, cmd := range newCommands {
		newCommandNames[cmd.Name()] = struct{}{}
	}

	// instead of root being constructed like in the commented out snippet, we
	// rebuild it the old way.
	root := cmd.NewRootCmd(client.New())

	// gather the slice of commands which must be replaced with their new
	// iterations
	var commandsToReplace []*cobra.Command
	for _, cmd := range root.Commands() {
		if _, exists := newCommandNames[cmd.Name()]; exists {
			commandsToReplace = append(commandsToReplace, cmd)
		}
	}

	// remove them
	root.RemoveCommand(commandsToReplace...)

	// make sure the remaining old commands run the preparers
	// TODO: remove when migration is done
	wrapRunE(root)

	// and finally, add the new commands
	root.AddCommand(newCommands...)

	return root
}

func wrapRunE(cmd *cobra.Command) {
	if cmd.HasAvailableSubCommands() {
		for _, c := range cmd.Commands() {
			wrapRunE(c)
		}
	}

	if cmd.RunE == nil && cmd.Run == nil {
		return
	}

	if cmd.RunE == nil {
		panic(cmd.Name())
	}

	cmd.RunE = command.WrapRunE(cmd.RunE)
}
