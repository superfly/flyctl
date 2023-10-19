package main

import (
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strings"
	"unicode"

	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/superfly/flyctl/internal/command/root"
)

func newLintCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "lint",
		Short: "Run checks against flyctl commands and flags",
		Run: func(cmd *cobra.Command, args []string) {
			root := root.New()
			run := NewCheckRun(root)

			table := tablewriter.NewWriter(os.Stdout)
			table.SetBorder(false)
			table.SetAutoWrapText(false)
			table.SetHeader([]string{"Path", "Check", "Failure Reason"})

			errors := run.Run()
			for _, err := range errors {
				table.Append([]string{
					err.command,
					err.check,
					err.Error(),
				})
			}

			table.Render()

			fmt.Println()
			fmt.Printf("%d checks failed\n", len(errors))

			if len(errors) > 0 {
				os.Exit(1)
			}
		},
	}
	return cmd
}

// add more to this list as needed. Just make sure they are actually common (in flyctl and other well known CLIs)!
var commonFlagShorthands = map[string]string{
	"app":          "a",
	"org":          "o",
	"config":       "c",
	"access-token": "t",
	"help":         "h",
	"json":         "j",
}

type run struct {
	rootCmd        *cobra.Command
	commandsByName map[string][]*cobra.Command
	flagsByName    map[string][]*pflag.Flag
}

func (r *run) Run() (errors []checkError) {
	walk(r.rootCmd, func(cmd *cobra.Command, depth int) {
		for name, fn := range commandChecks {
			if err := fn(r, cmd); err != nil {
				errors = append(errors, checkError{
					command: cmd.CommandPath(),
					check:   name,
					err:     err,
				})
			}
		}

		cmd.Flags().VisitAll(func(f *pflag.Flag) {
			for name, fn := range flagChecks {
				if err := fn(r, cmd, f); err != nil {
					errors = append(errors, checkError{
						command: cmd.CommandPath() + " --" + f.Name,
						check:   name,
						err:     err,
					})
				}
			}
		})
	})

	return
}

func NewCheckRun(root *cobra.Command) *run {
	r := &run{
		rootCmd:        root,
		commandsByName: make(map[string][]*cobra.Command),
		flagsByName:    make(map[string][]*pflag.Flag),
	}

	walk(root, func(cmd *cobra.Command, depth int) {
		r.commandsByName[cmd.Name()] = append(r.commandsByName[cmd.Name()], cmd)

		cmd.Flags().VisitAll(func(f *pflag.Flag) {
			r.flagsByName[f.Name] = append(r.flagsByName[f.Name], f)
		})
	})

	return r
}

type checkError struct {
	command string
	check   string
	err     error
}

func (e *checkError) Error() string {
	return e.err.Error()
}

type cmdCheckFn func(run *run, cmd *cobra.Command) error

var commandChecks = map[string]cmdCheckFn{
	"flags-in-usage":         redundantUsageCheck,
	"duplicate-description":  duplicateDescription,
	"description-too-long":   shortDescriptionTooLong,
	"inconsistent-aliases":   inconsistentAliases,
	"usage-punctuation":      usagePunctuation,
	"newline-in-description": newlineInDescription,
	"missing-descriptions":   missingDescriptions,
	"short-description-case": shortDescriptionCasing,
	"poor-command-name":      poorCommandName,
	"too-many-subcommands":   tooManySubcommands,
	"invalid-group":          invalidGroup,
}

type flagCheckFn func(run *run, cmd *cobra.Command, flag *pflag.Flag) error

var flagChecks = map[string]flagCheckFn{
	// disabled for now because there are a lot of these
	// "flag-punctuation": flagPunctuation,
	"misused-shorthand":            misusedShorthand,
	"inconsistent-flag-shorthands": inconsistentFlagShorthands,
}

func redundantUsageCheck(run *run, cmd *cobra.Command) error {
	if strings.Contains(cmd.Use, "[flags]") {
		return fmt.Errorf("redundant \"[flags]\" in usage string: \"%s\"; remove \"[flags]\"", cmd.Use)
	}
	return nil
}

func duplicateDescription(run *run, cmd *cobra.Command) error {
	if cmd.Long == cmd.Short {
		return fmt.Errorf("duplicate cmd.Long and cmd.Short; remove cmd.long")
	}
	return nil
}

func shortDescriptionCasing(run *run, cmd *cobra.Command) error {
	if cmd.Short != "" && !unicode.IsUpper([]rune(cmd.Short)[0]) {
		return fmt.Errorf("cmd.Short should be capitalized")
	}
	return nil
}

func newlineInDescription(run *run, cmd *cobra.Command) error {
	if strings.Contains(cmd.Short, "\n") {
		return fmt.Errorf("cmd.Short cannot contain newlines")
	}
	return nil
}

func missingDescriptions(run *run, cmd *cobra.Command) error {
	if cmd.Short == "" && cmd.Long == "" {
		return fmt.Errorf("no description; add cmd.Short and ideally cmd.Long")
	} else if cmd.Short == "" {
		return fmt.Errorf("has cmd.Long but not cmd.Short; add cmd.Short")
	}
	return nil
}

func shortDescriptionTooLong(run *run, cmd *cobra.Command) error {
	if len(cmd.Short) > 80 {
		return fmt.Errorf("cmd.Short is too long and risks wrapping; should be 80 characters or less")
	}
	return nil
}

func usagePunctuation(run *run, cmd *cobra.Command) error {
	if strings.HasSuffix(cmd.Use, ".") {
		return fmt.Errorf("cmd.Use should not end with a period")
	}
	return nil
}

func inconsistentAliases(run *run, cmd *cobra.Command) error {
	aliases := map[string]int{}
	cmdCount := len(run.commandsByName[cmd.Name()])
	for _, otherCmd := range run.commandsByName[cmd.Name()] {
		for _, alias := range otherCmd.Aliases {
			aliases[alias]++
		}
	}

	uncommon := []string{}
	missing := []string{}

	for alias, count := range aliases {
		if float64(count)/float64(cmdCount) > 0.5 {
			if !slices.Contains(cmd.Aliases, alias) {
				missing = append(missing, alias)
			}
		} else {
			if slices.Contains(cmd.Aliases, alias) {
				uncommon = append(uncommon, alias)
			}
		}
	}

	if len(uncommon) > 0 {
		return fmt.Errorf("has aliases not used by other commands with the same name: %s", encodeForMessage(uncommon))
	}

	if len(missing) > 0 {
		return fmt.Errorf("missing aliases used by other commands with the same name: %s", encodeForMessage(missing))
	}

	return nil
}

func poorCommandName(run *run, cmd *cobra.Command) error {
	var msg string
	switch cmd.Name() {
	case "destroy":
		msg = "is too agro. consider renaming to \"delete\""
	}

	if msg != "" {
		return fmt.Errorf("command name \"%s\" %s", cmd.Name(), msg)
	}
	return nil
}

func tooManySubcommands(run *run, cmd *cobra.Command) error {
	var count int
	for _, c := range cmd.Commands() {
		if !c.Hidden {
			count++
		}
	}

	if count > 10 && len(cmd.Groups()) > 0 {
		return fmt.Errorf("too many non-hidden subcommands (%d); consider using groups to organze them", count)
	}

	return nil
}

func invalidGroup(run *run, cmd *cobra.Command) error {
	if cmd.GroupID != "" && !cmd.Parent().ContainsGroup(cmd.GroupID) {
		groupIDs := []string{}
		for _, g := range cmd.Parent().Groups() {
			groupIDs = append(groupIDs, g.ID)
		}
		return fmt.Errorf("group \"%s\" is not registered on the parent command: %v", cmd.GroupID, groupIDs)
	}
	return nil
}

// func flagPunctuation(run *run, cmd *cobra.Command, flag *pflag.Flag) error {
// 	if !strings.HasSuffix(flag.Usage, ".") {
// 		return fmt.Errorf("flag.Usage should end with a period")
// 	}
// 	return nil
// }

func misusedShorthand(run *run, cmd *cobra.Command, flag *pflag.Flag) error {
	if flag.Shorthand != "" {
		for name, shorthand := range commonFlagShorthands {
			if flag.Shorthand == shorthand && flag.Name != name {
				return fmt.Errorf("shorthand \"%s\" should only be used with flags named \"%s\"", shorthand, name)
			}
		}
	}

	return nil
}

func inconsistentFlagShorthands(run *run, cmd *cobra.Command, flag *pflag.Flag) error {
	shorthandCounts := map[string]int{}
	flagCount := len(run.flagsByName[flag.Name])
	for _, otherFlags := range run.flagsByName[flag.Name] {
		shorthandCounts[otherFlags.Shorthand]++
	}

	for shorthand, count := range shorthandCounts {
		if float64(count)/float64(flagCount) > 0.5 {
			if flag.Shorthand != shorthand {
				return fmt.Errorf("shorthand \"%s\" is inconsistent with other flags with the same name: %v", flag.Shorthand, encodeForMessage(shorthandCounts))
			}
		}
	}

	return nil
}

func encodeForMessage(x any) string {
	b, err := json.Marshal(x)
	if err != nil {
		panic(err)
	}
	return string(b)
}
