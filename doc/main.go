package main

// This is a basic docs gen with cobra's md_docs.go and util.go brought in now to give complete control over
// document generation - Dj

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/internal/cli"
)

var titlePrefix = "## "

// pageName is the file name a command is published under, without extension.
// The root command is published as "fly_help", which is the name the old
// Sitepress pipeline gave it. 42 published pages link to it that way and
// docs.json redirects into it, so renaming it would mean breaking live URLs or
// adding redirects to fix a file name. Not worth either.
func pageName(name string) string {
	base := strings.TrimSuffix(filepath.Base(name), path.Ext(name))
	if base == "fly" {
		return "fly_help"
	}

	return base
}

// escapeMDX guards prose that MDX would read as JSX. Only "<" needs it, and
// only outside code fences: 69 published pages carry a raw "<" inside a fence
// with no trouble, while the four in prose are escaped. An unescaped "<" in
// prose is worse than a build error, because Mintlify drops an unknown
// component silently and the text just disappears.
func escapeMDX(s string) string {
	return strings.ReplaceAll(s, "<", "\\<")
}

func main() {
	cmd := cli.NewRootCommand()
	cmd.DisableAutoGenTag = true

	// Override root command to always be `fly`,
	// Otherwise it could be `main`, `flyctl` or whatever name is set to the executable
	cmd.Use = "fly"

	// The slug these pages had on the old Sitepress site. Still used for the
	// source_url frontmatter, which every imported docs page carries.
	legacySlug := func(name string) string {
		base := strings.TrimSuffix(filepath.Base(name), path.Ext(name))
		base = strings.Replace(base, "fly_", "", 1)
		if base == "fly" {
			base = "help"
		}
		base = strings.ReplaceAll(base, "_", "-") + "/"

		return strings.ToLower(base)
	}

	// Mintlify reads title from frontmatter. sidebarTitle is deliberately not
	// emitted: on 36 pages it is a hand-written nav label that cobra's Short
	// does not match, so scripts/publish_docs.sh carries the existing value
	// through instead of this overwriting it.
	//
	// description is emitted after source_url so the sidebarTitle carried
	// across by scripts/generate_docs.sh still lands on line 3.
	filePrepender := func(cmd *cobra.Command, filename string) string {
		return fmt.Sprintf("---\ntitle: %s\nsource_url: https://fly.io/docs/flyctl/%s\ndescription: %s\n---\n\n",
			cmd.CommandPath(), legacySlug(filename), quoteYAML(description(cmd)))
	}

	// Pages live at /flyctl/cmd/<file basename> on docs.fly.io, so the link is
	// the file name rather than the old hyphenated slug.
	linkHandler := func(name string) string {
		return "/flyctl/cmd/" + pageName(name)
	}

	os.MkdirAll("out", 0o700)

	err := GenMarkdownTreeCustom(cmd, "./out", filePrepender, linkHandler)
	if err != nil {
		log.Fatal(err)
	}
}

// quoteYAML double-quotes s for frontmatter. Go's escapes are a subset of
// what YAML double-quoted scalars accept, and quoting is needed anyway since
// every description contains ": ".
func quoteYAML(s string) string {
	return strconv.Quote(s)
}

// maxDescriptionLen keeps the meta description inside what Google shows in a
// result snippet. Without a description Mintlify emits no meta description at
// all and search engines write their own snippet from the page body.
const maxDescriptionLen = 155

// description builds the page's meta description from the command's Short
// help, e.g. "Reference for fly apps move: move an app to another
// organization. Usage and flags." Each fallback drops the least useful part
// until it fits: the trailing section list, then the "Reference for" lead-in,
// then the end of Short itself.
func description(cmd *cobra.Command) string {
	path := cmd.CommandPath()
	short := strings.TrimRight(strings.Join(strings.Fields(cmd.Short), " "), ".")
	if short == "" {
		return truncateWords("Reference for "+path+": usage and flags.", maxDescriptionLen)
	}
	short = lowerFirstWord(short)

	var sections string
	switch {
	case hasSubCommands(cmd) && len(cmd.Example) > 0:
		sections = "Subcommands, usage, flags and examples."
	case hasSubCommands(cmd):
		sections = "Subcommands, usage and flags."
	case len(cmd.Example) > 0:
		sections = "Usage, flags and examples."
	default:
		sections = "Usage and flags."
	}

	for _, d := range []string{
		"Reference for " + path + ": " + short + ". " + sections,
		"Reference for " + path + ": " + short + ".",
		path + ": " + short + ".",
	} {
		if len(d) <= maxDescriptionLen {
			return d
		}
	}

	return truncateWords(path+": "+short, maxDescriptionLen)
}

// lowerFirstWord lowercases a leading plain capitalized word ("Move an app"
// becomes "move an app") so Short reads naturally after a colon. Acronyms,
// "[experimental]" and product names are left alone.
func lowerFirstWord(s string) string {
	word, _, _ := strings.Cut(s, " ")
	word = strings.TrimRight(word, ",")
	switch word {
	case "Fly", "Unmanaged":
		return s
	}
	if len(word) < 2 || word[0] < 'A' || word[0] > 'Z' {
		return s
	}
	for _, r := range word[1:] {
		if r < 'a' || r > 'z' {
			return s
		}
	}

	return strings.ToLower(s[:1]) + s[1:]
}

// truncateWords cuts s at a word boundary so that it plus an ellipsis fits in
// limit bytes.
func truncateWords(s string, limit int) string {
	if len(s) <= limit {
		return s
	}
	cut := s[:limit-3]
	if i := strings.LastIndex(cut, " "); i > 0 {
		cut = cut[:i]
	}

	return strings.TrimRight(cut, " ,.:;") + "..."
}

func printOptions(buf *bytes.Buffer, cmd *cobra.Command, name string) error {
	flags := cmd.NonInheritedFlags()
	flags.SetOutput(buf)
	if flags.HasAvailableFlags() {
		buf.WriteString(titlePrefix + "Options\n\n```\n")
		flags.PrintDefaults()
		buf.WriteString("```\n\n")
	}

	parentFlags := cmd.InheritedFlags()
	parentFlags.SetOutput(buf)
	if parentFlags.HasAvailableFlags() {
		buf.WriteString(titlePrefix + "Global Options\n\n```\n")
		parentFlags.PrintDefaults()
		buf.WriteString("```\n\n")
	}

	return nil
}

// GenMarkdownCustom creates custom markdown output.
func GenMarkdownCustom(cmd *cobra.Command, w io.Writer, linkHandler func(string) string) error {
	cmd.InitDefaultHelpCmd()
	cmd.InitDefaultHelpFlag()

	buf := new(bytes.Buffer)
	name := cmd.CommandPath()
	// name := cmd.Name()

	short := cmd.Short
	long := cmd.Long
	if len(long) == 0 {
		long = short
	}

	buf.WriteString(escapeMDX(long) + "\n\n")

	if len(cmd.UseLine()) > 0 {
		buf.WriteString(titlePrefix + "Usage\n")

		// If it's runnable, show the useline otherwise show a version with [command]
		if cmd.Runnable() {
			fmt.Fprintf(buf, "```\n%s\n```\n\n", cmd.UseLine())
		} else {
			fmt.Fprintf(buf, "```\n%s [command] [flags]\n```\n\n", cmd.CommandPath())
		}
	}

	if hasSubCommands(cmd) {
		buf.WriteString(titlePrefix + "Available Commands\n")
		children := cmd.Commands()
		sort.Sort(byName(children))

		for _, child := range children {
			if !child.IsAvailableCommand() || child.IsAdditionalHelpTopicCommand() {
				continue
			}
			cname := name + " " + child.Name()
			link := cname + ".md"
			link = strings.ReplaceAll(link, " ", "_")
			fmt.Fprintf(buf, "* [%s](%s)\t - %s\n", child.Name(), linkHandler(link), escapeMDX(child.Short))
		}
		buf.WriteString("\n")
	}

	if len(cmd.Example) > 0 {
		buf.WriteString(titlePrefix + "Examples\n\n")
		fmt.Fprintf(buf, "```\n%s\n```\n\n", cmd.Example)
	}

	if err := printOptions(buf, cmd, name); err != nil {
		return err
	}
	if hasSeeAlso(cmd) {
		buf.WriteString(titlePrefix + "See Also\n\n")
		if cmd.HasParent() {
			parent := cmd.Parent()
			pname := parent.CommandPath()
			link := pname + ".md"
			link = strings.ReplaceAll(link, " ", "_")
			fmt.Fprintf(buf, "* [%s](%s)\t - %s\n", pname, linkHandler(link), escapeMDX(parent.Short))
			cmd.VisitParents(func(c *cobra.Command) {
				if c.DisableAutoGenTag {
					cmd.DisableAutoGenTag = c.DisableAutoGenTag
				}
			})
		}

		// children := cmd.Commands()
		// sort.Sort(byName(children))

		// for _, child := range children {
		// 	if !child.IsAvailableCommand() || child.IsAdditionalHelpTopicCommand() {
		// 		continue
		// 	}
		// 	cname := name + " " + child.Name()
		// 	link := cname + ".md"
		// 	link = strings.Replace(link, " ", "_", -1)
		// 	buf.WriteString(fmt.Sprintf("* [%s](%s)\t - %s\n", cname, linkHandler(link), child.Short))
		// }
		buf.WriteString("\n")
	}
	if !cmd.DisableAutoGenTag {
		buf.WriteString("###### Auto generated by spf13/cobra on " + time.Now().Format("2-Jan-2006") + "\n")
	}

	// Match the shape of the pages already published on docs.fly.io, which the
	// Mintlify import left without a blank line after a closing fence and
	// without a trailing blank line. Without this the first sync after the
	// MDX switch rewrites all 300-odd pages with whitespace-only diffs and the
	// real changes are lost in the noise.
	out := strings.ReplaceAll(buf.String(), "```\n\n", "```\n")
	out = strings.TrimRight(out, "\n") + "\n"

	_, err := io.WriteString(w, out)

	return err
}

// GenMarkdownTreeCustom is the the same as GenMarkdownTree, but
// with custom filePrepender and linkHandler.
func GenMarkdownTreeCustom(cmd *cobra.Command, dir string, filePrepender func(*cobra.Command, string) string, linkHandler func(string) string) error {
	for _, c := range cmd.Commands() {
		if !c.IsAvailableCommand() || c.IsAdditionalHelpTopicCommand() {
			continue
		}
		if err := GenMarkdownTreeCustom(c, dir, filePrepender, linkHandler); err != nil {
			return err
		}
	}

	basename := pageName(strings.ReplaceAll(cmd.CommandPath(), " ", "_")) + ".mdx"
	filename := filepath.Join(dir, basename)
	f, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer f.Close()

	if _, err := io.WriteString(f, filePrepender(cmd, filename)); err != nil {
		return err
	}
	if err := GenMarkdownCustom(cmd, f, linkHandler); err != nil {
		return err
	}

	return nil
}

// Test to see if we have a reason to print See Also information in docs
// Basically this is a test for a parent commend or a subcommand which is
// both not deprecated and not the autogenerated help command.
func hasSeeAlso(cmd *cobra.Command) bool {
	if cmd.HasParent() {
		return true
	}
	for _, c := range cmd.Commands() {
		if !c.IsAvailableCommand() || c.IsAdditionalHelpTopicCommand() {
			continue
		}

		return true
	}

	return false
}

// Test to see if we have a reason to print Sub Commands information in docs
// Basically this is a test for a parent commend or a subcommand which is
// both not deprecated and not the autogenerated help command.
func hasSubCommands(cmd *cobra.Command) bool {
	for _, c := range cmd.Commands() {
		if !c.IsAvailableCommand() || c.IsAdditionalHelpTopicCommand() {
			continue
		}

		return true
	}

	return false
}

type byName []*cobra.Command

func (s byName) Len() int           { return len(s) }
func (s byName) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }
func (s byName) Less(i, j int) bool { return s[i].Name() < s[j].Name() }
