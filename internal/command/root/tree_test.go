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

// rootCmd and tree, its visible commands, are built once in TestMain.
var (
	rootCmd *cobra.Command
	tree    []visibleCommand
)

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

		rootCmd = New()
		tree = walkTree(rootCmd)

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
// rule applies to (nil means every visible command); problem says what a
// command in scope has to change to follow the rule, or returns "" if it
// already does.
type rule struct {
	inScope func(c *cobra.Command) bool
	problem func(c *cobra.Command) string
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

		problem := r.problem(vc.cmd)
		switch {
		case problem != "" && !allowed[vc.path]:
			newFailures = append(newFailures, fmt.Sprintf("%q: %s", vc.path, problem))
		case problem == "" && allowed[vc.path]:
			t.Errorf("%q now passes this rule: delete it from the allowlist", vc.path)
		}
	}

	for p := range allowed {
		if !visible[p] {
			t.Errorf("allowlist entry %q is not a visible command: delete it", p)
		}
	}

	sort.Strings(newFailures)
	for _, failure := range newFailures {
		t.Error(failure)
	}
}

func TestCommandTree_HasShortDescription(t *testing.T) {
	checkRule(t, nil, rule{problem: func(c *cobra.Command) string {
		if strings.TrimSpace(c.Short) == "" {
			return "add a Short"
		}

		return ""
	}})
}

var (
	urlPattern  = regexp.MustCompile(`https?://\S+`)
	wordPattern = regexp.MustCompile(`[a-z0-9]+(?:['’_-][a-z0-9]+)*`)
)

// words lowercases s and splits it into words, dropping punctuation and URLs.
// A contraction, a flag or a hyphenated word counts as one word.
func words(s string) []string {
	return wordPattern.FindAllString(urlPattern.ReplaceAllString(strings.ToLower(s), " "), -1)
}

// extraWords counts the words in long that aren't accounted for by short.
func extraWords(long, short string) int {
	inShort := map[string]int{}
	for _, w := range words(short) {
		inShort[w]++
	}

	extra := 0
	for _, w := range words(long) {
		if inShort[w] > 0 {
			inShort[w]--
		} else {
			extra++
		}
	}

	return extra
}

// minExtraLongWords is how many words a Long must add beyond its Short.
const minExtraLongWords = 8

// A Long that only restates Short gives an agent reading --help nothing more,
// so it must add at least a sentence of its own.
func TestCommandTree_HasLongDescription(t *testing.T) {
	checkRule(t, missingLongAllowlist, rule{problem: func(c *cobra.Command) string {
		if extra := extraWords(c.Long, c.Short); extra < minExtraLongWords {
			return fmt.Sprintf("Long adds %d words to Short, want at least %d (URLs don't count)", extra, minExtraLongWords)
		}

		return ""
	}})
}

func TestExtraWords(t *testing.T) {
	const short = "Show the one and only status of a widget, in some detail"
	for _, tc := range []struct {
		name, long string
		want       int
	}{
		{"empty", "", 0},
		{"same words", "show the ONE and only status of a widget in some detail.", 0},
		{"short plus a link", short + "\n\nSee https://fly.io/docs/reference/configuration/#the-widget-section", 1},
		{"short plus a sentence", short + ". Widgets that don't exist are left out of --json output.", 10},
		{"mostly new", "Print its current state, size and region for each process group", 10},
		{"repeats a word of short", short + " status status", 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := extraWords(tc.long, short); got != tc.want {
				t.Errorf("got %d, want %d", got, tc.want)
			}
		})
	}
}

// destructiveVerbs are words that, in the name or an alias of a command or of
// one of its parents, mark it as destroying or detaching something.
var destructiveVerbs = map[string]bool{
	"destroy": true, "delete": true, "remove": true, "rm": true,
	"revoke": true, "release": true, "unset": true, "reset": true,
	"kill": true, "detach": true, "clear": true, "purge": true, "prune": true,
}

func isDestructive(c *cobra.Command) bool {
	for ; c.HasParent(); c = c.Parent() {
		for _, name := range append([]string{c.Name()}, c.Aliases...) {
			for _, word := range strings.Split(name, "-") {
				if destructiveVerbs[word] {
					return true
				}
			}
		}
	}

	return false
}

// offersFlag reports whether c accepts the flag, from itself or a parent, and
// shows it in --help. It uses Flag rather than Flags().Lookup, which only sees
// a persistent flag once something else has made cobra merge it.
func offersFlag(c *cobra.Command, name string) bool {
	f := c.Flag(name)

	return f != nil && !f.Hidden && f.Deprecated == ""
}

// missingFlag returns the problem of not offering the flag, or "".
func missingFlag(c *cobra.Command, name string) string {
	if offersFlag(c, name) {
		return ""
	}

	return fmt.Sprintf("add a visible --%s flag", name)
}

// A destructive command must offer --yes, so scripts and agents can always
// pass it, whether or not the command prompts today.
func TestCommandTree_DestructiveCommandsHaveYes(t *testing.T) {
	checkRule(t, destructiveWithoutYesAllowlist, rule{
		inScope: func(c *cobra.Command) bool { return c.Runnable() && isDestructive(c) },
		problem: func(c *cobra.Command) string { return missingFlag(c, flagnames.Yes) },
	})
}

// Every command that does something itself, including a group that also runs
// on its own, should be able to return its result as JSON.
func TestCommandTree_HasJSONOutput(t *testing.T) {
	checkRule(t, missingJSONAllowlist, rule{
		inScope: func(c *cobra.Command) bool { return c.Runnable() },
		problem: func(c *cobra.Command) string { return missingFlag(c, flagnames.JSONOutput) },
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
			f := vc.cmd.Flag(name)
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

// nonKebabFlags returns the problem of c's own flags with names that aren't
// kebab-case, or "".
func nonKebabFlags(c *cobra.Command) string {
	var names []string
	c.LocalFlags().VisitAll(func(f *pflag.Flag) {
		if !f.Hidden && f.Deprecated == "" && !kebabCase.MatchString(f.Name) {
			names = append(names, "--"+f.Name)
		}
	})
	if len(names) == 0 {
		return ""
	}

	return "rename to kebab-case: " + strings.Join(names, ", ")
}

func TestCommandTree_FlagNamesAreKebabCase(t *testing.T) {
	checkRule(t, nonKebabFlagAllowlist, rule{problem: nonKebabFlags})

	// The tree leaves out the root command, and a command's local flags leave
	// out those it inherits, so check the global flags here.
	if problem := nonKebabFlags(rootCmd); problem != "" {
		t.Errorf("global flags: %s", problem)
	}
}

// The rules must see a flag inherited from a parent, whichever test runs
// first, and must not be satisfied by a flag --help doesn't show.
func TestOffersFlag(t *testing.T) {
	build := func(change func(parent, child *cobra.Command)) *cobra.Command {
		parent, child := &cobra.Command{Use: "parent"}, &cobra.Command{Use: "child"}
		parent.AddCommand(child)
		change(parent, child)

		return child
	}

	for _, tc := range []struct {
		name   string
		want   bool
		change func(parent, child *cobra.Command)
	}{
		{"none", false, func(_, _ *cobra.Command) {}},
		{"own", true, func(_, child *cobra.Command) { child.Flags().Bool("json", false, "") }},
		{"inherited", true, func(parent, _ *cobra.Command) { parent.PersistentFlags().Bool("json", false, "") }},
		{"parent's own", false, func(parent, _ *cobra.Command) { parent.Flags().Bool("json", false, "") }},
		{"hidden", false, func(_, child *cobra.Command) {
			child.Flags().Bool("json", false, "")
			_ = child.Flags().MarkHidden("json")
		}},
		{"deprecated", false, func(_, child *cobra.Command) {
			child.Flags().Bool("json", false, "")
			_ = child.Flags().MarkDeprecated("json", "gone")
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := offersFlag(build(tc.change), "json"); got != tc.want {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestIsDestructiveCoversSubcommands(t *testing.T) {
	root, revoke := &cobra.Command{Use: "clear"}, &cobra.Command{Use: "revoke"}
	supplied, list := &cobra.Command{Use: "supplied"}, &cobra.Command{Use: "list"}
	root.AddCommand(revoke, list)
	revoke.AddCommand(supplied)

	if !isDestructive(supplied) {
		t.Error("a subcommand of revoke isn't destructive")
	}
	// The root's own name doesn't count.
	if isDestructive(list) {
		t.Error("list is destructive")
	}
}
