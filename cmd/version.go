package cmd

import (
	"fmt"
	"github.com/superfly/flyctl/cmdctx"
	"os"

	"github.com/superfly/flyctl/docstrings"

	"github.com/superfly/flyctl/flyctl"
)

func newVersionCommand() *Command {
	versionStrings := docstrings.Get("version")
	version := BuildCommand(nil, runVersion, versionStrings.Usage, versionStrings.Short, versionStrings.Long, os.Stdout)
	version.AddStringFlag(StringFlagOpts{
		Name:        "completions",
		Shorthand:   "c",
		Description: "Generate completions for supported shells bash/zsh)",
	})
	return version
}

type FlyctlBuild struct {
	Version string
	Commit  string
	Build   string
}

func runVersion(ctx *cmdctx.CmdContext) error {
	shellType, _ := ctx.Config.GetString("completions")

	if shellType != "" {
		switch shellType {
		case "bash":
			return GetRootCommand().GenBashCompletion(os.Stdout)
		case "zsh":
			return GetRootCommand().GenZshCompletion(os.Stdout)
		default:
			return fmt.Errorf("unable to generate %s completions", shellType)
		}
	}

	if ctx.OutputJSON() {
		ctx.WriteJSON(FlyctlBuild{Version: flyctl.Version, Commit: flyctl.Commit, Build: flyctl.BuildDate})
	} else {
		fmt.Printf("flyctl %s %s %s\n", flyctl.Version, flyctl.Commit, flyctl.BuildDate)
	}
	return nil
}
