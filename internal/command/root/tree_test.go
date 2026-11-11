package root

import (
	"fmt"
	"os"
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

// tree is built once in TestMain.
var tree []visibleCommand

// TestMain builds the command tree once, with HOME and FLY_CONFIG_DIR pointed
// at a temporary directory: root.New loads config from disk, and the tests
// must neither read nor write the developer's real ~/.fly.
func TestMain(m *testing.M) {
	os.Exit(func() int {
		dir, err := os.MkdirTemp("", "flyctl-tree-test")
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		defer os.RemoveAll(dir)

		os.Setenv("HOME", dir)
		os.Setenv("FLY_CONFIG_DIR", dir+"/.fly")

		tree = walkTree(New())

		return m.Run()
	}())
}

func walkTree(root *cobra.Command) []visibleCommand {
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

	return out
}

func visibleCommands(t *testing.T) []visibleCommand {
	t.Helper()

	if len(tree) < 100 {
		t.Fatalf("walked only %d commands; the tree walk is broken", len(tree))
	}

	return tree
}

// rule is one check over the command tree. inScope picks the commands the
// rule applies to (nil means every visible command); pass reports whether a
// command in scope follows the rule.
type rule struct {
	inScope func(c *cobra.Command) bool
	pass    func(c *cobra.Command) bool
}

// checkRule compares the commands that break r with allowlist. A failing
// command that isn't allowlisted fails the test, and so does an allowlisted
// command that now passes, is out of the rule's scope, or no longer exists.
func checkRule(t *testing.T, allowlist []string, r rule) {
	t.Helper()

	allowed := map[string]bool{}
	for _, p := range allowlist {
		if allowed[p] {
			t.Errorf("allowlist lists %q twice", p)
		}
		allowed[p] = true
	}

	var newFailures []string
	visible := map[string]bool{}
	for _, vc := range visibleCommands(t) {
		visible[vc.path] = true

		if r.inScope != nil && !r.inScope(vc.cmd) {
			if allowed[vc.path] {
				t.Errorf("%q is allowlisted but this rule no longer applies to it: check the change that moved it out of scope still meets the rule's intent, then delete the entry", vc.path)
			}
			continue
		}

		ok := r.pass(vc.cmd)
		switch {
		case !ok && !allowed[vc.path]:
			newFailures = append(newFailures, vc.path)
		case ok && allowed[vc.path]:
			t.Errorf("%q now passes this rule: delete it from the allowlist", vc.path)
		}
	}

	for p := range allowed {
		if !visible[p] {
			t.Errorf("allowlist entry %q is not a visible command: delete it", p)
		}
	}

	sort.Strings(newFailures)
	for _, p := range newFailures {
		t.Errorf("%q breaks this rule", p)
	}
}

func TestCommandTree_HasShortDescription(t *testing.T) {
	checkRule(t, nil, rule{pass: func(c *cobra.Command) bool {
		return strings.TrimSpace(c.Short) != ""
	}})
}

var nonWord = regexp.MustCompile(`[^a-z0-9]+`)

// words lowercases s and splits it into words, dropping punctuation.
func words(s string) []string {
	return strings.Fields(nonWord.ReplaceAllString(strings.ToLower(s), " "))
}

// minExtraLongWords is how many words a Long must add beyond its Short.
const minExtraLongWords = 8

// A Long that only restates Short gives an agent reading --help nothing more,
// so it must add at least a sentence of its own.
func TestCommandTree_HasLongDescription(t *testing.T) {
	checkRule(t, missingLongAllowlist, rule{pass: func(c *cobra.Command) bool {
		long, short := words(c.Long), words(c.Short)
		if strings.Join(long, " ") == strings.Join(short, " ") {
			return false
		}

		return len(long) >= len(short)+minExtraLongWords
	}})
}

// destructiveVerbs are words that, in a command's name or one of its
// aliases, mark it as destroying or detaching something.
var destructiveVerbs = map[string]bool{
	"destroy": true, "delete": true, "remove": true, "rm": true,
	"revoke": true, "release": true, "unset": true, "reset": true,
	"kill": true, "detach": true, "clear": true, "purge": true, "prune": true,
}

func isDestructive(c *cobra.Command) bool {
	for _, name := range append([]string{c.Name()}, c.Aliases...) {
		for _, word := range strings.Split(name, "-") {
			if destructiveVerbs[word] {
				return true
			}
		}
	}

	return false
}

// A destructive command must offer --yes, so scripts and agents can always
// pass it, whether or not the command prompts today.
func TestCommandTree_DestructiveCommandsHaveYes(t *testing.T) {
	checkRule(t, destructiveWithoutYesAllowlist, rule{
		inScope: func(c *cobra.Command) bool { return c.Runnable() && isDestructive(c) },
		pass:    func(c *cobra.Command) bool { return c.Flags().Lookup(flagnames.Yes) != nil },
	})
}

// Every command that does something itself, including a group that also runs
// on its own, should be able to return its result as JSON.
func TestCommandTree_HasJSONOutput(t *testing.T) {
	checkRule(t, missingJSONAllowlist, rule{
		inScope: func(c *cobra.Command) bool { return c.Runnable() },
		pass:    func(c *cobra.Command) bool { return c.Flags().Lookup(flagnames.JSONOutput) != nil },
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
	checkRule(t, nonKebabFlagAllowlist, rule{pass: func(c *cobra.Command) bool {
		ok := true
		c.LocalFlags().VisitAll(func(f *pflag.Flag) {
			if !f.Hidden && f.Deprecated == "" && !kebabCase.MatchString(f.Name) {
				ok = false
			}
		})
		return ok
	}})
}
