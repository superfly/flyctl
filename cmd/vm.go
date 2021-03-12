package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/cmdctx"
	"github.com/superfly/flyctl/docstrings"
	"github.com/superfly/flyctl/internal/client"
)

func newVMCommand(client *client.Client) *Command {
	vmCmd := BuildCommandKS(nil, nil, docstrings.Get("vm"), client)

	vmRestartCmd := BuildCommandKS(vmCmd, runVMRestart, docstrings.Get("vm.restart"), client, requireSession, requireAppName)
	vmRestartCmd.Args = cobra.ExactArgs(1)

	vmStopCmd := BuildCommandKS(vmCmd, runVMStop, docstrings.Get("vm.stop"), client, requireSession, requireAppName)
	vmStopCmd.Args = cobra.ExactArgs(1)

	vmStatusCmd := BuildCommandKS(vmCmd, runAllocStatus, docstrings.Get("vm.status"), client, requireSession, requireAppName)
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
