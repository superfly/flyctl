package root

import (
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/superfly/flyctl/internal/flag/flagnames"
)

// These tests walk the whole command tree and check rules every command must
// follow, so a new command can't quietly break them. Commands that break a
// rule today are listed in tree_allowlist_test.go. Fix a command, then delete
// its entry: an entry for a command that now passes fails the test, so the
// lists only shrink.

// visibleCommand is a non-hidden, non-deprecated command, keyed by its path
// without the root name (e.g. "machine destroy").
type visibleCommand struct {
	path string
	cmd  *cobra.Command
}

func visibleCommands(t *testing.T) []visibleCommand {
	t.Helper()

	root := New()

	var out []visibleCommand
	var walk func(c *cobra.Command, hiddenParent bool)
	walk = func(c *cobra.Command, hiddenParent bool) {
		hidden := hiddenParent || c.Hidden || c.Deprecated != ""
		if c != root && !hidden && c.Name() != "help" && c.Name() != "completion" {
			path := strings.TrimPrefix(c.CommandPath(), root.Name()+" ")
			out = append(out, visibleCommand{path: path, cmd: c})
		}
		for _, sub := range c.Commands() {
			walk(sub, hidden)
		}
	}
	walk(root, false)

	if len(out) < 100 {
		t.Fatalf("walked only %d commands; the tree walk is broken", len(out))
	}

	return out
}

// isRunnable reports whether c does something itself rather than only
// grouping subcommands.
func isRunnable(c *cobra.Command) bool {
	return c.Runnable() && !c.HasAvailableSubCommands()
}

// checkRule runs rule over every command and compares the commands that fail
// it with allowlist. A failing command that isn't allowlisted, and an
// allowlisted command that now passes or no longer exists, both fail the test.
func checkRule(t *testing.T, cmds []visibleCommand, allowlist []string, rule func(c *cobra.Command) bool) {
	t.Helper()

	allowed := map[string]bool{}
	for _, p := range allowlist {
		if allowed[p] {
			t.Errorf("allowlist lists %q twice", p)
		}
		allowed[p] = true
	}

	var newFailures []string
	seen := map[string]bool{}
	for _, vc := range cmds {
		seen[vc.path] = true
		ok := rule(vc.cmd)
		switch {
		case !ok && !allowed[vc.path]:
			newFailures = append(newFailures, vc.path)
		case ok && allowed[vc.path]:
			t.Errorf("%q now passes this rule: delete it from the allowlist", vc.path)
		}
	}

	for p := range allowed {
		if !seen[p] {
			t.Errorf("allowlist entry %q is not a visible command: delete it", p)
		}
	}

	sort.Strings(newFailures)
	for _, p := range newFailures {
		t.Errorf("%q breaks this rule", p)
	}
}

func TestCommandTree_HasShortDescription(t *testing.T) {
	cmds := visibleCommands(t)
	checkRule(t, cmds, nil, func(c *cobra.Command) bool {
		return strings.TrimSpace(c.Short) != ""
	})
}

// A Long that only repeats Short gives an agent reading --help nothing more.
func TestCommandTree_HasLongDescription(t *testing.T) {
	cmds := visibleCommands(t)
	checkRule(t, cmds, missingLongAllowlist, func(c *cobra.Command) bool {
		long := strings.TrimSpace(c.Long)
		return long != "" && long != strings.TrimSpace(c.Short)
	})
}

var destructiveVerbs = map[string]bool{
	"destroy": true, "delete": true, "remove": true, "rm": true,
	"revoke": true, "release": true, "unset": true, "reset": true,
}

// A destructive command that prompts must offer --yes, or it can't run
// without a terminal.
func TestCommandTree_DestructiveCommandsHaveYes(t *testing.T) {
	var cmds []visibleCommand
	for _, vc := range visibleCommands(t) {
		if isRunnable(vc.cmd) && destructiveVerbs[vc.cmd.Name()] {
			cmds = append(cmds, vc)
		}
	}
	checkRule(t, cmds, destructiveWithoutYesAllowlist, func(c *cobra.Command) bool {
		return c.Flags().Lookup(flagnames.Yes) != nil
	})
}

// Every runnable command should be able to return its result as JSON.
func TestCommandTree_HasJSONOutput(t *testing.T) {
	var cmds []visibleCommand
	for _, vc := range visibleCommands(t) {
		if isRunnable(vc.cmd) {
			cmds = append(cmds, vc)
		}
	}
	checkRule(t, cmds, missingJSONAllowlist, func(c *cobra.Command) bool {
		return c.Flags().Lookup(flagnames.JSONOutput) != nil
	})
}

// Where --json and --yes exist they mean the same thing everywhere: a boolean
// switch with its standard shorthand or none.
func TestCommandTree_StandardFlagsAreUniform(t *testing.T) {
	standard := map[string]string{
		flagnames.JSONOutput: "j",
		flagnames.Yes:        "y",
	}

	for _, vc := range visibleCommands(t) {
		for name, short := range standard {
			f := vc.cmd.Flags().Lookup(name)
			if f == nil {
				continue
			}
			if f.Value.Type() != "bool" {
				t.Errorf("%q: --%s is a %s, want bool", vc.path, name, f.Value.Type())
			}
			if f.Shorthand != "" && f.Shorthand != short {
				t.Errorf("%q: --%s has shorthand -%s, want -%s or none", vc.path, name, f.Shorthand, short)
			}
		}
	}
}

var kebabCase = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

func TestCommandTree_FlagNamesAreKebabCase(t *testing.T) {
	cmds := visibleCommands(t)
	checkRule(t, cmds, nonKebabFlagAllowlist, func(c *cobra.Command) bool {
		ok := true
		c.LocalFlags().VisitAll(func(f *pflag.Flag) {
			if !f.Hidden && f.Deprecated == "" && !kebabCase.MatchString(f.Name) {
				ok = false
			}
		})
		return ok
	})
}
