package cmd

import (
	"context"
	"fmt"
	"path/filepath"
	"runtime"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/cmdctx"
	"github.com/superfly/flyctl/flyctl"
	"github.com/superfly/flyctl/flyname"
	"github.com/superfly/flyctl/internal/client"
	"github.com/superfly/flyctl/internal/update"

	"github.com/superfly/flyctl/docstrings"
)

func newVersionCommand(client *client.Client) *Command {
	versionStrings := docstrings.Get("version")
	version := BuildCommandKS(nil, runVersion, versionStrings, client)
	version.AddStringFlag(StringFlagOpts{
		Name:        "saveinstall",
		Shorthand:   "s",
		Description: "Save parameter in config",
	})
	version.Flag("saveinstall").Hidden = true

	updateStrings := docstrings.Get("version.update")
	BuildCommandKS(version, runUpdate, updateStrings, client)

	initStateCmd := BuildCommand(version, runInitState, "init-state", "init-state", "Initialize installation state", client)
	initStateCmd.Hidden = true
	initStateCmd.Args = cobra.ExactArgs(1)

	return version
}

func runVersion(ctx *cmdctx.CmdContext) error {
	saveInstall := ctx.Config.GetString("saveinstall")

	if saveInstall != "" {
		stateFilePath := filepath.Join(flyctl.ConfigDir(), "state.yml")
		update.InitState(stateFilePath, saveInstall)
	}

	if ctx.OutputJSON() {
		type flyctlBuild struct {
			Name         string
			Version      string
			Commit       string
			BuildDate    string
			OS           string
			Architecture string
		}
		ctx.WriteJSON(flyctlBuild{Name: flyname.Name(), Version: flyctl.Version, Commit: flyctl.Commit, BuildDate: flyctl.BuildDate, OS: runtime.GOOS, Architecture: runtime.GOARCH})
	} else {
		fmt.Printf("%s v%s %s/%s Commit: %s BuildDate: %s\n", flyname.Name(), flyctl.Version, runtime.GOOS, runtime.GOARCH, flyctl.Commit, flyctl.BuildDate)
	}

	return nil
}

func runInitState(ctx *cmdctx.CmdContext) error {
	stateFilePath := filepath.Join(flyctl.ConfigDir(), "state.yml")
	return update.InitState(stateFilePath, ctx.Args[0])
}

func runUpdate(ctx *cmdctx.CmdContext) error {
	stateFilePath := filepath.Join(flyctl.ConfigDir(), "state.yml")
	return update.PerformInPlaceUpgrade(context.TODO(), stateFilePath, flyctl.Version)
}
