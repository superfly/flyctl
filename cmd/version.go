package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/viper"
	"github.com/superfly/flyctl/cmdctx"

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
	version.AddStringFlag(StringFlagOpts{
		Name:        "saveinstall",
		Shorthand:   "s",
		Description: "Save parameter in config",
	})
	version.Flag("saveinstall").Hidden = true

	return version
}

func runVersion(ctx *cmdctx.CmdContext) error {
	saveInstall, _ := ctx.Config.GetString("saveinstall")

	if saveInstall != "" {
		fmt.Println(saveInstall)
		viper.Set(flyctl.ConfigInstaller, saveInstall)
		if err := flyctl.SaveConfig(); err != nil {
			return err
		}
		return nil
	}

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
		type flyctlBuild struct {
			Version string
			Commit  string
			Build   string
		}
		ctx.WriteJSON(flyctlBuild{Version: flyctl.Version, Commit: flyctl.Commit, Build: flyctl.BuildDate})
	} else {
		fmt.Printf("flyctl %s %s %s\n", flyctl.Version, flyctl.Commit, flyctl.BuildDate)
	}
	return nil
}
