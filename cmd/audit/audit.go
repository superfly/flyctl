package main

import (
	"log"

	"github.com/spf13/cobra"
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "audit",
		Short: "Tool for auditing flyctl commands",
	}

	rootCmd.AddCommand(newListCmd())
	rootCmd.AddCommand(newStatsCmd())
	rootCmd.AddCommand(newLintCmd())

	if err := rootCmd.Execute(); err != nil {
		log.Fatalln(err)
	}
}

type walkFn func(cmd *cobra.Command, depth int)

func walkInternal(cmd *cobra.Command, maxDepth int, depth int, fn walkFn) {
	if maxDepth > -1 && depth > maxDepth {
		return
	}

	fn(cmd, depth)

	for _, childCmd := range cmd.Commands() {
		walkInternal(childCmd, maxDepth, depth+1, fn)
	}
}

func walk(cmd *cobra.Command, fn walkFn) {
	walkInternal(cmd, -1, 0, fn)
}

func walkMaxDepth(cmd *cobra.Command, maxDepth int, fn walkFn) {
	walkInternal(cmd, maxDepth, 0, fn)
}
