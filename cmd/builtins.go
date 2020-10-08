package cmd

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/logrusorgru/aurora"
	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"
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
	builtinShowAppStrings := docstrings.Get("builtins.show-app")
	showappcmd := BuildCommandKS(cmd, runShowAppBuiltin, builtinShowAppStrings, os.Stdout, requireAppName)
	showappcmd.Args = cobra.MaximumNArgs(0)

	return cmd
}

func runListBuiltins(commandContext *cmdctx.CmdContext) error {
	builtins := builtinsupport.GetBuiltins(commandContext)

	sort.Slice(builtins, func(i, j int) bool { return builtins[i].Name < builtins[j].Name })

	builtintable := tablewriter.NewWriter(commandContext.Out)
	builtintable.SetHeader([]string{"Name", "Description", "Details"})
	builtintable.SetHeaderAlignment(tablewriter.ALIGN_LEFT)
	builtintable.SetAlignment(tablewriter.ALIGN_LEFT)
	builtintable.SetNoWhiteSpace(true)
	builtintable.SetTablePadding(" ")
	builtintable.SetCenterSeparator("")
	builtintable.SetColumnSeparator("")
	builtintable.SetRowSeparator("")

	for _, builtin := range builtins {
		builtintable.Append([]string{builtin.Name, builtin.Description, builtin.Details})
		builtintable.Append([]string{"", "", ""})
	}

	builtintable.Render()

	return nil
}

func runShowAppBuiltin(commandContext *cmdctx.CmdContext) error {
	builtinname := commandContext.AppConfig.Build.Builtin
	return showBuiltin(commandContext, builtinname, true)
}

func runShowBuiltin(commandContext *cmdctx.CmdContext) error {

	if len(commandContext.Args) == 0 {
		builtinname, err := selectBuiltin(commandContext)
		if err != nil {
			return err
		}
		return showBuiltin(commandContext, builtinname, false)

	}

	builtinname := commandContext.Args[0]

	return showBuiltin(commandContext, builtinname, false)
}

func showBuiltin(commandContext *cmdctx.CmdContext, builtinname string, useSettings bool) error {
	builtin, err := builtinsupport.GetBuiltin(commandContext, builtinname)
	if err != nil {
		return err
	}

	if commandContext.OutputJSON() {
		commandContext.WriteJSON(builtin)
		return nil
	}

	showBuiltinMetadata(commandContext, builtin)

	var settings map[string]interface{}

	if useSettings {
		settings = builtin.ResolveSettings(commandContext.AppConfig.Build.Settings)
	} else {
		settings = builtin.ResolveSettings(nil)
	}

	if len(settings) > 0 {
		fmt.Print(aurora.Bold("Settings:\n"))
		for name, val := range settings {
			setting := builtin.GetSetting(name)
			valType := ""
			formattedVal := ""
			switch val.(type) {
			case []interface{}:
			case []string:
				valType = "array"
				semi := fmt.Sprintf("%q", val)
				tokens := strings.Split(semi, " ")
				formattedVal = strings.Join(tokens, ",")
			case bool:
				valType = "bool"
				formattedVal = fmt.Sprintf("%t", val)
			default:
				valType = fmt.Sprintf("%T", val)
				formattedVal = fmt.Sprintf("%s", val)
			}
			if useSettings {
				fmt.Printf("%s=%s (%s)\n     %s defaults to %s\n", name, formattedVal, valType, setting.Description, setting.Default)
			} else {
				fmt.Printf("%s=%s (%s)\n     %s\n", name, formattedVal, valType, setting.Description)
			}
		}
		fmt.Println()
	}

	if commandContext.AppConfig == nil {
		fmt.Println(aurora.Bold("Dockerfile (with defaults):"))
		vdockerfile, err := builtin.GetVDockerfile(nil)
		if err != nil {
			return err
		}
		fmt.Println(vdockerfile)
	} else {
		fmt.Println(aurora.Bold("Dockerfile (with fly.toml settins):"))
		vdockerfile, err := builtin.GetVDockerfile(commandContext.AppConfig.Build.Settings)
		if err != nil {
			return err
		}
		fmt.Println(vdockerfile)
	}

	return nil
}

func showBuiltinMetadata(commandContext *cmdctx.CmdContext, builtin *builtinsupport.Builtin) {
	fmt.Print(aurora.Bold("Name: "))
	fmt.Println(builtin.Name)
	fmt.Println()

	fmt.Print(aurora.Bold("Description: "))
	fmt.Println(builtin.Description)
	fmt.Println()

	fmt.Print(aurora.Bold("Details:\n"))
	fmt.Println(builtin.Details)
	fmt.Println()
}
