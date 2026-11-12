package root

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flyerr"
)

// Groups with their own runners must keep their existing argument contracts.
// Keep this explicit so a newly added help-only group cannot escape the walk.
func TestUnknownSubcommands(t *testing.T) {
	t.Setenv("FLY_CONFIG_DIR", t.TempDir())
	runnable := map[string]bool{
		"fly": true, "fly dashboard": true, "fly doctor": true,
		"fly jobs": true, "fly launch": true, "fly settings analytics": true,
		"fly settings autoupdate": true, "fly settings synthetics": true,
		"fly skills": true, "fly tokens revoke": true, "fly version": true,
	}
	root := New()
	root.Use = "fly"
	root.InitDefaultCompletionCmd()
	var walk func(*cobra.Command)
	walk = func(cmd *cobra.Command) {
		if cmd.HasSubCommands() && !runnable[cmd.CommandPath()] {
			if cmd.Runnable() {
				t.Fatalf("group became runnable: %s; add it to the runnable list if intended", cmd.CommandPath())
			}
			path := strings.Fields(cmd.CommandPath())[1:]
			t.Run(strings.Join(path, "/"), func(t *testing.T) {
				fresh := New()
				fresh.Use = "fly"
				fresh.SilenceErrors = true
				var stdout, stderr bytes.Buffer
				fresh.SetOut(&stdout)
				fresh.SetErr(&stderr)
				fresh.SetArgs(append(path, "definitely-not-a-command"))
				_, err := ExecuteContextC(context.Background(), fresh)
				resolved, _, _ := fresh.Find(path)
				if resolved.Run != nil || resolved.RunE != nil {
					t.Fatalf("help-only group gained a runner: %s", resolved.CommandPath())
				}
				want := fmt.Sprintf("unknown command %q for %q", "definitely-not-a-command", cmd.CommandPath())
				if err == nil || !strings.Contains(err.Error(), want) {
					t.Fatalf("expected %s; got %v", want, err)
				}
				if !strings.Contains(flyerr.GetErrorSuggestion(err), "Run '"+cmd.CommandPath()+" --help' for usage.") {
					t.Fatalf("missing help hint: %v", err)
				}
				wantStdout := ""
				if resolved.Deprecated != "" {
					// Cobra prints this notice for every invocation of a deprecated command.
					wantStdout = fmt.Sprintf("Command %q is deprecated, %s\n", resolved.Name(), resolved.Deprecated)
				}
				if stdout.String() != wantStdout {
					t.Fatalf("unexpected stdout: %s", &stdout)
				}
			})
		}
		for _, child := range cmd.Commands() {
			walk(child)
		}
	}
	walk(root)
}

func TestGroupHelpAndSuggestions(t *testing.T) {
	t.Setenv("FLY_CONFIG_DIR", t.TempDir())
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"mpg", "lst"}, "Did you mean this?\n\tlist"},
		{[]string{"mpg"}, ""},
	} {
		t.Run(strings.Join(tc.args, " "), func(t *testing.T) {
			cmd := New()
			cmd.Use = "fly"
			cmd.SilenceErrors = true
			var out, stderr bytes.Buffer
			cmd.SetOut(&out)
			cmd.SetErr(&stderr)
			cmd.SetArgs(tc.args)
			_, err := ExecuteContextC(context.Background(), cmd)
			if tc.want == "" {
				if err != nil || !strings.Contains(out.String(), "Usage:") {
					t.Fatalf("help: %v %s", err, &out)
				}
			} else if err == nil || !strings.Contains(fmt.Sprint(err), tc.want) {
				t.Fatalf("error: %v", err)
			}
			if tc.want != "" && out.Len() != 0 {
				t.Fatalf("unexpected stdout: %s", &out)
			}
		})
	}
}

func TestRunnableGroupsKeepPositionalArguments(t *testing.T) {
	t.Setenv("FLY_CONFIG_DIR", t.TempDir())
	for _, path := range [][]string{
		{"tokens", "revoke"}, {"version"}, {"jobs"}, {"settings", "analytics"},
	} {
		t.Run(strings.Join(path, "/"), func(t *testing.T) {
			root := New()
			cmd, _, err := root.Find(path)
			if err != nil {
				t.Fatal(err)
			}
			// Replace only the runner to avoid preparers that require an account.
			called := false
			cmd.RunE = func(_ *cobra.Command, args []string) error {
				called = true
				if len(args) != 1 || args[0] != "positional-argument" {
					t.Fatalf("args: %v", args)
				}
				return nil
			}
			root.SetArgs(append(path, "positional-argument"))
			if _, err := ExecuteContextC(context.Background(), root); err != nil || !called {
				t.Fatalf("runner called=%v, error=%v", called, err)
			}
		})
	}
}

func TestGroupFlagPlacement(t *testing.T) {
	t.Setenv("FLY_CONFIG_DIR", t.TempDir())
	for _, tc := range []struct {
		args string
		want string
	}{
		{"mpg --org acme bogus", `unknown command "bogus" for "fly mpg"`},
		{"mpg bogus --help", ""},
		{"-t x mpg bogus", `unknown command "bogus" for "fly mpg"`},
		{"mpg -- bogus", `unknown command "bogus" for "fly mpg"`},
		{"help mpg bogus", ""},
		{"completion bogus", `unknown command "bogus" for "fly completion"`},
		{"completion -- bogus", `unknown command "bogus" for "fly completion"`},
		{"mpg --help users --org acme", ""},
		{"mpg --help list --json", "unknown flag: --json"},
		{"mpg -h list --json", "unknown flag: --json"},
		{"mpg --help users", ""}, {"mpg -h list", ""},
		{"mpg help", ""}, {"apps help", ""}, {"mpg help list", ""},
		{"mpg helpp", `unknown command "helpp" for "fly mpg"`},
		{"mpg lst --json", "unknown flag: --json"},
	} {
		t.Run(tc.args, func(t *testing.T) {
			cmd := New()
			cmd.Use = "fly"
			cmd.SilenceErrors = true
			var out bytes.Buffer
			cmd.SetOut(&out)
			cmd.SetErr(&out)
			args := strings.Fields(tc.args)
			cmd.SetArgs(args)
			_, err := ExecuteContextC(context.Background(), cmd)
			if tc.want == "" {
				if err != nil || !strings.Contains(out.String(), "Usage:") {
					t.Fatalf("help: %v %s", err, &out)
				}
			} else if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want %s, got %v", tc.want, err)
			}
			if strings.HasPrefix(tc.want, "unknown command") && out.Len() != 0 {
				t.Fatalf("unexpected output: %s", &out)
			}
			if tc.want == "unknown flag: --json" && !strings.Contains(out.String(), "Usage:") {
				t.Fatalf("missing usage: %s", &out)
			}
		})
	}
}

func TestGroupHelpUnchanged(t *testing.T) {
	t.Setenv("FLY_CONFIG_DIR", t.TempDir())
	var walk func(*cobra.Command)
	walk = func(group *cobra.Command) {
		if group.HasSubCommands() && !group.Runnable() {
			path := strings.Fields(group.CommandPath())[1:]
			for _, args := range [][]string{path, append(append([]string{}, path...), "--help"), append([]string{"help"}, path...)} {
				t.Run(strings.Join(args, " "), func(t *testing.T) {
					var outputs [2]string
					for i := range outputs {
						cmd := New()
						cmd.Use = "fly"
						var out bytes.Buffer
						cmd.SetOut(&out)
						cmd.SetErr(&out)
						cmd.SetArgs(args)
						var err error
						if i == 0 {
							_, err = cmd.ExecuteContextC(context.Background())
						} else {
							_, err = ExecuteContextC(context.Background(), cmd)
						}
						if err != nil {
							t.Fatal(err)
						}
						outputs[i] = out.String()
						resolved, _, _ := cmd.Find(path)
						if resolved.Runnable() {
							t.Fatalf("group became runnable: %s", resolved.CommandPath())
						}
					}
					if outputs[0] != outputs[1] {
						t.Fatalf("help changed:\nbefore:\n%s\nafter:\n%s", outputs[0], outputs[1])
					}
				})
			}
		}
		for _, child := range group.Commands() {
			walk(child)
		}
	}
	walk(New())
}

func TestGroupSuggestionSettings(t *testing.T) {
	t.Setenv("FLY_CONFIG_DIR", t.TempDir())
	for _, tc := range []struct {
		disabled bool
		distance int
		word     string
		suggest  bool
	}{
		{false, 0, "lst", true}, {true, 0, "lst", false},
		{false, 0, "", false},
		{false, 1, "lxt", false}, {false, 2, "lxt", true},
	} {
		cmd := New()
		group, _, _ := cmd.Find([]string{"mpg"})
		group.DisableSuggestions = tc.disabled
		group.SuggestionsMinimumDistance = tc.distance
		args := []string{"mpg", tc.word}
		cmd.SetArgs(args)
		_, err := ExecuteContextC(context.Background(), cmd)
		if err == nil || strings.Contains(err.Error(), "Did you mean this?") != tc.suggest {
			t.Fatalf("%+v: %v", tc, err)
		}
	}
}

func TestGroupFlagValuesAndDeprecationWarningPreserved(t *testing.T) {
	cmd := &cobra.Command{Use: "fly"}
	group := &cobra.Command{Use: "group"}
	group.AddCommand(&cobra.Command{Use: "child", Run: func(*cobra.Command, []string) {}})
	cmd.AddCommand(group)
	values := group.Flags().StringArray("value", nil, "values")
	group.Flags().String("old", "", "old")
	if err := group.Flags().MarkDeprecated("old", "use value"); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	args := []string{"group", "bogus", "--value", "one", "--value", "two", "--old", "x"}
	cmd.SetArgs(args)
	_, err := ExecuteContextC(context.Background(), cmd)
	if err == nil || strings.Join(*values, ",") != "one,two" || strings.Count(out.String(), "deprecated") != 1 {
		t.Fatalf("err=%v values=%v output=%s", err, *values, &out)
	}
}

func TestHelpResolutionUnchanged(t *testing.T) {
	t.Setenv("FLY_CONFIG_DIR", t.TempDir())
	for _, invocation := range []string{
		"mpg --help users --org acme", "mpg --help list --json", "mpg -h list --json",
		"mpg --help users", "mpg -h list", "mpg help", "apps help", "mpg help list",
		"completion bash", "completion fish", "help completion bogus",
		"__complete", "__complete mpg lst", "__completeNoDesc", "__completeNoDesc mpg lst",
	} {
		t.Run(invocation, func(t *testing.T) {
			var outputs [2]string
			var errors [2]string
			for i := range outputs {
				cmd := New()
				cmd.Use = "fly"
				var out bytes.Buffer
				cmd.SetOut(&out)
				cmd.SetErr(&out)
				args := strings.Fields(invocation)
				cmd.SetArgs(args)
				var err error
				if i == 0 {
					_, err = cmd.ExecuteContextC(context.Background())
				} else {
					_, err = ExecuteContextC(context.Background(), cmd)
				}
				outputs[i] = out.String()
				errors[i] = fmt.Sprint(err)
			}
			if outputs[0] != outputs[1] || errors[0] != errors[1] {
				t.Fatalf("baseline: %s %s\nwrapped: %s %s", errors[0], outputs[0], errors[1], outputs[1])
			}
		})
	}
}

func TestGroupExtraArgs(t *testing.T) {
	t.Setenv("FLY_CONFIG_DIR", t.TempDir())
	cmd := New()
	cmd.Use = "fly"
	args := []string{"launch", "plan"}
	cmd.SetArgs(args)
	var out bytes.Buffer
	cmd.SetOut(&out)
	ctx := flag.WithExtraArgs(context.Background(), []string{"bogus"})
	_, err := ExecuteContextC(ctx, cmd)
	if err == nil || !strings.Contains(err.Error(), `unknown command "bogus" for "fly launch plan"`) || out.Len() != 0 {
		t.Fatalf("err=%v output=%s", err, &out)
	}
}
