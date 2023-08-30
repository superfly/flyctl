package scale

import (
	"context"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
)

func newScaleMemory() *cobra.Command {
	const (
		short = "Set VM memory"
		long  = `Set VM memory to a number of megabytes`
	)
	cmd := command.New("memory [memoryMB]", short, long, runScaleMemory,
		command.RequireSession,
		command.RequireAppName,
	)
	cmd.Args = cobra.ExactArgs(1)
	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
		flag.String{Name: "group", Description: "The process group to apply the VM size to"},
	)
	return cmd
}

func runScaleMemory(ctx context.Context) error {
	group := flag.GetString(ctx, "group")

	memoryMB := parseMemory(flag.FirstArg(ctx))

	return scaleVertically(ctx, group, "", int(memoryMB))
}

func parseMemory(memory string) int {
	// Find the index where the numeric part ends and the unit part begins
	i := strings.IndexFunc(memory, func(r rune) bool { return r < '0' || r > '9' })

	// Parse the numeric part to an integer
	number, _ := strconv.Atoi(memory[:i])

	// The rest of the string is the unit
	unit := memory[i:]

	switch unit {
	case "GB", "gb":
		number *= 1024
	}

	return number
}
