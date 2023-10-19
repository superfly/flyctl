package main

import (
	"os"
	"slices"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/superfly/flyctl/internal/command/root"
)

func formatRawText(desc string) string {
	return strings.ReplaceAll(desc, "\n", "⏎")
}

func newListCmd() *cobra.Command {
	var maxDepth int
	var printUsage bool
	var printDescription bool
	var outputFormat string
	var flags bool

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List flyctl commands",
		RunE: func(cmd *cobra.Command, args []string) error {
			output := newOutput(os.Stdout, outputFormat)
			root := root.New()

			header := []string{"Command", "Aliases", "Flags", "Details"}
			if flags {
				header = slices.Insert(header, 1, "Flag")
			}
			if printUsage {
				header = append(header, "Usage")
			}
			if printDescription {
				header = append(header, "Description")
			}
			output.SetHeader(header)

			walkMaxDepth(root, maxDepth, func(cmd *cobra.Command, depth int) {
				cols := []string{
					cmd.CommandPath(),
					strings.Join(cmd.Aliases, ", "),
					strconv.Itoa(countFlags(cmd)),
					commandAttrs(cmd),
				}

				if flags {
					cols = slices.Insert(cols, 1, "")
				}

				if printUsage {
					cols = append(cols, formatRawText(cmd.UseLine()))
				}
				if printDescription {
					cols = append(cols, formatRawText(cmd.Short))
				}

				output.Append(cols)

				if flags {
					cmd.Flags().VisitAll(func(f *pflag.Flag) {
						cols = []string{
							cmd.CommandPath(),
							"--" + f.Name,
							f.Shorthand,
							"",
							flagAttrs(f),
						}
						if printUsage {
							cols = append(cols, f.Usage)
						}
						if printDescription {
							cols = append(cols, "")
						}

						output.Append(cols)
					})
				}
			})

			return output.Flush()
		},
	}
	cmd.Flags().IntVar(&maxDepth, "max-depth", -1, "Maximum depth to walk the command tree")
	cmd.Flags().BoolVar(&printUsage, "usage", false, "Print usage for each command")
	cmd.Flags().BoolVar(&printDescription, "description", false, "Print description for each command")
	cmd.Flags().StringVar(&outputFormat, "output", "table", "Output format (table, csv, json)")
	cmd.Flags().BoolVar(&flags, "flags", false, "Include flags for each command")

	return cmd
}

func commandAttrs(cmd *cobra.Command) string {
	attrs := []string{}
	if cmd.GroupID != "" {
		attrs = append(attrs, "group:"+cmd.GroupID)
	}
	if cmd.Deprecated != "" {
		attrs = append(attrs, "deprecated")
	}
	if cmd.Hidden {
		attrs = append(attrs, "hidden")
	}
	if !cmd.Runnable() {
		attrs = append(attrs, "not_runnable")
	}
	if cmd.HasExample() {
		attrs = append(attrs, "has_example")
	}
	slices.Sort(attrs)
	return strings.Join(attrs, ", ")
}

func flagAttrs(f *pflag.Flag) string {
	attrs := []string{}
	if f.Deprecated != "" {
		attrs = append(attrs, "deprecated")
	}
	if f.Hidden {
		attrs = append(attrs, "hidden")
	}
	if f.ShorthandDeprecated != "" {
		attrs = append(attrs, "shorthand_deprecated")
	}
	slices.Sort(attrs)
	return strings.Join(attrs, ", ")
}

func countFlags(cmd *cobra.Command) (count int) {
	cmd.Flags().VisitAll(func(f *pflag.Flag) {
		count++
	})
	return
}
