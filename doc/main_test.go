package main

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/superfly/flyctl/internal/cli"
)

func TestDescription(t *testing.T) {
	root := &cobra.Command{Use: "fly", Short: "The Fly.io command line interface"}
	apps := &cobra.Command{Use: "apps", Short: "Manage apps."}
	move := &cobra.Command{Use: "move", Short: "Move an app to another organization", Run: func(*cobra.Command, []string) {}}
	ssh := &cobra.Command{Use: "ssh", Short: "SSH into a Machine", Example: "fly ssh", Run: func(*cobra.Command, []string) {}}
	pg := &cobra.Command{Use: "postgres", Short: "Unmanaged Postgres cluster commands"}
	long := &cobra.Command{Use: "long", Short: strings.Repeat("word ", 60), Run: func(*cobra.Command, []string) {}}
	root.AddCommand(apps, ssh, pg, long)
	apps.AddCommand(move)
	pg.AddCommand(&cobra.Command{Use: "list", Short: "list clusters", Run: func(*cobra.Command, []string) {}})

	for _, tc := range []struct {
		cmd  *cobra.Command
		want string
	}{
		{move, "Reference for fly apps move: move an app to another organization. Usage and flags."},
		{apps, "Reference for fly apps: manage apps. Subcommands, usage and flags."},
		{ssh, "Reference for fly ssh: SSH into a Machine. Usage, flags and examples."},
		{pg, "Reference for fly postgres: Unmanaged Postgres cluster commands. Subcommands, usage and flags."},
		{root, "Reference for fly: the Fly.io command line interface. Subcommands, usage and flags."},
	} {
		if got := description(tc.cmd); got != tc.want {
			t.Errorf("description(%q)\n got %q\nwant %q", tc.cmd.CommandPath(), got, tc.want)
		}
	}

	if got := description(long); len(got) > maxDescriptionLen || !strings.HasPrefix(got, "fly long: ") || !strings.HasSuffix(got, "...") {
		t.Errorf("long Short not truncated cleanly: %q (%d)", got, len(got))
	}
}

// Every generated page must get a description that fits in a search snippet,
// names its command, and survives as YAML frontmatter.
func TestDescriptionAllCommands(t *testing.T) {
	root := cli.NewRootCommand()
	root.Use = "fly"

	seen := map[string]string{}
	var walk func(c *cobra.Command)
	walk = func(c *cobra.Command) {
		if c != root && (!c.IsAvailableCommand() || c.IsAdditionalHelpTopicCommand()) {
			return
		}
		d := description(c)
		if len(d) > maxDescriptionLen {
			t.Errorf("%s: description is %d bytes: %q", c.CommandPath(), len(d), d)
		}
		if !strings.Contains(d, c.CommandPath()+":") {
			t.Errorf("%s: description does not name the command: %q", c.CommandPath(), d)
		}

		var fm struct {
			Title       string `yaml:"title"`
			Description string `yaml:"description"`
		}
		block := "title: " + c.CommandPath() + "\ndescription: " + quoteYAML(d) + "\n"
		if err := yaml.Unmarshal([]byte(block), &fm); err != nil || fm.Description != d {
			t.Errorf("%s: frontmatter does not round-trip: %v %q", c.CommandPath(), err, fm.Description)
		}

		if other, ok := seen[d]; ok {
			t.Logf("%s and %s share a description: %q", c.CommandPath(), other, d)
		}
		seen[d] = c.CommandPath()

		for _, child := range c.Commands() {
			walk(child)
		}
	}
	walk(root)
}
