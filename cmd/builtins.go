package cmd

import (
	"os"
	"sort"

	"github.com/olekukonko/tablewriter"
	"github.com/superfly/flyctl/builtinsupport"
	"github.com/superfly/flyctl/cmdctx"

	"github.com/superfly/flyctl/docstrings"

	"github.com/spf13/cobra"
)

func newBuiltinsCommand() *Command {
	builtinsStrings := docstrings.Get("builtins")

	cmd := &Command{
		Command: &cobra.Command{
			Use:   builtinsStrings.Usage,
			Short: builtinsStrings.Short,
			Long:  builtinsStrings.Long,
		},
	}

	builtinsListStrings := docstrings.Get("builtins.list")
	BuildCommandKS(cmd, runListBuiltins, builtinsListStrings, os.Stdout)

	return cmd
}

func runListBuiltins(commandContext *cmdctx.CmdContext) error {
	builtins := builtinsupport.GetBuiltins()

	sort.Slice(builtins, func(i, j int) bool { return builtins[i].Name < builtins[j].Name })

	builtintable := tablewriter.NewWriter(commandContext.Out)
	builtintable.SetHeader([]string{"Name", "Description", "Details"})

	for _, builtin := range builtins {
		builtintable.Append([]string{builtin.Name, builtin.Description, builtin.Details})
		builtintable.Append([]string{"", "", ""})
	}

	builtintable.Render()

	return nil
}
