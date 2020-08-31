package cmd

import (
	"fmt"
	"os"
	"sort"

	"github.com/logrusorgru/aurora"
	"github.com/olekukonko/tablewriter"
	"github.com/superfly/flyctl/builtinsupport"
	"github.com/superfly/flyctl/cmdctx"

	"github.com/superfly/flyctl/docstrings"
)

func newBuiltinsCommand() *Command {
	builtinsStrings := docstrings.Get("builtins")

	cmd := BuildCommandKS(nil, nil, builtinsStrings, os.Stdout)

	builtinsListStrings := docstrings.Get("builtins.list")
	BuildCommandKS(cmd, runListBuiltins, builtinsListStrings, os.Stdout)
	builtinShowStrings := docstrings.Get("builtins.show")
	BuildCommandKS(cmd, runShowBuiltin, builtinShowStrings, os.Stdout)

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

func runShowBuiltin(commandContext *cmdctx.CmdContext) error {
	var builtinname string
	var err error

	if len(commandContext.Args) == 0 {
		builtinname, err = selectBuiltin(commandContext)
		if err != nil {
			return err
		}
	} else {
		builtinname = commandContext.Args[0]
	}

	builtin, err := builtinsupport.GetBuiltin(builtinname)

	if err != nil {
		return err
	}

	if commandContext.OutputJSON() {
		commandContext.WriteJSON(builtin)
		return nil
	}

	commandContext.Statusf("builtins", cmdctx.STITLE, "Name: %s\n", builtin.Name)
	commandContext.StatusLn()

	commandContext.Statusf("builtins", cmdctx.SINFO, "Description: %s\n", builtin.Description)
	commandContext.StatusLn()

	fmt.Print(aurora.Bold("Details:\n"))
	fmt.Println(builtin.Details)
	fmt.Println()

	fmt.Print(aurora.Bold("Dockerfile:\n"))
	fmt.Println(builtin.FileText)

	return nil
}
