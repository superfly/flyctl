package help

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"

	"github.com/olekukonko/tablewriter"
)

var deprecatedCommands = map[string]bool{
	"completion":  true,
	"curl":        true,
	"dns-records": true,
	"domains":     true,
}

func New(root *cobra.Command) *cobra.Command {
	cmd := command.New("help", "Help on flyctl commands", "", Help(root))

	list := command.New("commands", "All flyctl commands", "", HelpCommands(root))
	flag.Add(list, flag.Bool{
		Name:        "all",
		Shorthand:   "a",
		Default:     false,
		Description: "show all commands, even the ones we secretly hate.",
	})

	cmd.AddCommand(list)

	return cmd
}

// the output of `flyctl`, run by itself with no args
func NewRootHelp() *cobra.Command {
	return command.New("", "", "", func(ctx context.Context) error {
		auth := `

It doesn't look like you're logged in. Try "flyctl auth signup" to create an account,
or "flyctl auth login" to log in to an existing account.`

		if client.FromContext(ctx).Authenticated() {
			auth = ""
		}

		fmt.Printf(`This is flyctl, the Fly.io command line interface.%s

Here's a few commands to get you started:
  fly launch      Launch a new application
  fly apps        Create and manage apps
  fly postgres    Create and manage Postgres databases
  fly redis       Create and manage Redis databases
  fly machines    Create and manage individual Fly.io machines

If you need help along the way:
  fly help            Display a complete list of commands
  fly help <command>  Display help for a specific command, e.g. 'fly help launch'

Visit https://fly.io/docs for additional documentation & guides
`, auth)
		return nil
	})
}

// the output of `flyctl help`, possibly with more arguments
func Help(root *cobra.Command) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		if cmd, _, err := root.Find(flag.Args(ctx)); err == nil && cmd != root {
			return cmd.Help()
		}

		commands := map[string]*cobra.Command{}

		for _, cmd := range root.Commands() {
			cmd := cmd
			commands[cmd.Name()] = cmd
		}

		listCommands := func(names []string) {
			for _, name := range names {
				fmt.Printf("  %s %s\n", tablewriter.PadRight(name, " ", 15), commands[name].Short)
			}
		}

		fmt.Printf(`
Deploying apps and machines:
`)
		listCommands([]string{"apps", "machine", "launch", "deploy", "restart", "destroy", "open"})

		fmt.Printf(`
Scaling and configuring:
`)
		listCommands([]string{"scale", "regions", "secrets"})

		fmt.Printf(`
Provisioning storage:
`)
		listCommands([]string{"volumes", "postgres", "redis"})

		fmt.Printf(`
Networking configuration:
`)
		listCommands([]string{"ips", "wireguard", "proxy", "certs"})

		fmt.Printf(`
Monitoring and managing things:
`)
		listCommands([]string{"logs", "list", "status", "dashboard", "dig", "ping", "ssh", "sftp"})

		fmt.Printf(`
Access control:
`)
		listCommands([]string{"orgs", "auth", "move"})

		fmt.Printf(`
More help:
`)
		listCommands([]string{"docs", "doctor"})
		fmt.Printf("  help commands   A complete list of commands (there are a bunch more)\n")

		return nil
	}
}

// the output of `flyctl help commands`; the master list of commands
func HelpCommands(root *cobra.Command) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		all := flag.GetBool(ctx, "all")

		fmt.Printf("flyctl commands:\n")
		for _, cmd := range root.Commands() {
			if cmd.Hidden {
				continue
			}

			name := cmd.Name()
			if deprecatedCommands[name] && !all {
				continue
			}

			fmt.Printf("  %s %s\n", tablewriter.PadRight(name, " ", 15), cmd.Short)
		}

		fmt.Printf(`
Flags:
  -a, --all           List all flyctl commands, even the ones we secretly hate.
`)

		return nil
	}
}
