package main

import (
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/command/root"
)

func newStatsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "stats",
		Short: "Stats. Everybody loves stats.",
		Run: func(cmd *cobra.Command, args []string) {
			root := root.New()
			stats := stats{}

			walk(root, func(cmd *cobra.Command, depth int) {
				stats.Commands.Total++

				if cmd.Hidden {
					stats.Commands.Hidden++
				}

				if cmd.Deprecated != "" {
					stats.Commands.Deprecated++
				}

				if cmd.Runnable() {
					stats.Commands.Runnable++
				}

				if cmd.HasExample() {
					stats.Commands.WithExamples++
				}

				if command.IsAppsV1Command(cmd) {
					stats.Commands.AppsV1++
				}

				stats.Commands.Aliases += len(cmd.Aliases)

				cmd.Flags().VisitAll(func(f *pflag.Flag) {
					stats.Flags.Total++

					if f.Hidden {
						stats.Flags.Hidden++
					}

					if f.Deprecated != "" {
						stats.Flags.Deprecated++
					}

					if f.Shorthand != "" {
						stats.Flags.WithShorthand++
					}
				})
			})

			stats.Root.Groups = len(root.Groups())

			for _, cmd := range root.Commands() {
				stats.Root.Commands++

				if cmd.Hidden {
					stats.Root.Hidden++
				} else {
					stats.Root.Visible++
				}

				if cmd.Deprecated != "" {
					stats.Root.Deprecated++
				}

				if cmd.Runnable() {
					stats.Root.Runnable++
				}
			}

			prettyPrintJSON(stats)
		},
	}

	return cmd
}

type stats struct {
	Root struct {
		Commands   int
		Hidden     int
		Visible    int
		Deprecated int
		Runnable   int
		Groups     int
	}
	Commands struct {
		Total        int
		Runnable     int
		Hidden       int
		Deprecated   int
		WithExamples int
		Aliases      int
		AppsV1       int
	}
	Flags struct {
		Total         int
		Hidden        int
		Deprecated    int
		WithShorthand int
		AppsV1        int
	}
}
