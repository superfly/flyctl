package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/cmdctx"
	"github.com/superfly/flyctl/docstrings"
)

func newVMCommand() *Command {
	vmCmd := BuildCommandKS(nil, nil, docstrings.Get("vm"), os.Stdout)

	vmRestartCmd := BuildCommandKS(vmCmd, runVMRestart, docstrings.Get("vm.restart"), os.Stdout, requireSession, requireAppName)
	vmRestartCmd.Args = cobra.ExactArgs(1)

	vmStopCmd := BuildCommandKS(vmCmd, runVMStop, docstrings.Get("vm.stop"), os.Stdout, requireSession, requireAppName)
	vmStopCmd.Args = cobra.ExactArgs(1)

	vmStatusCmd := BuildCommandKS(vmCmd, runAllocStatus, docstrings.Get("vm.status"), os.Stdout, requireSession, requireAppName)
	vmStatusCmd.Args = cobra.ExactArgs(1)

	return vmCmd
}

func runVMRestart(cmdctx *cmdctx.CmdContext) error {
	appName := cmdctx.AppName
	allocID := cmdctx.Args[0]

	err := cmdctx.Client.API().RestartAllocation(appName, allocID)
	if err != nil {
		return err
	}

	fmt.Printf("VM %s is being restarted\n", allocID)
	return nil
}

func runVMStop(cmdctx *cmdctx.CmdContext) error {
	appName := cmdctx.AppName
	allocID := cmdctx.Args[0]

	err := cmdctx.Client.API().StopAllocation(appName, allocID)
	if err != nil {
		return err
	}

	fmt.Printf("VM %s is being stopped\n", allocID)
	return nil
}
