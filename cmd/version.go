package cmd

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/spf13/viper"
	"github.com/superfly/flyctl/cmdctx"
	"github.com/superfly/flyctl/flyctl"
	"github.com/superfly/flyctl/flyname"

	"github.com/superfly/flyctl/docstrings"
)

func newVersionCommand() *Command {
	versionStrings := docstrings.Get("version")
	version := BuildCommandKS(nil, runVersion, versionStrings, os.Stdout)
	version.AddBoolFlag(BoolFlagOpts{
		Name:        "full",
		Shorthand:   "f",
		Description: "Show full version details",
	})

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

	updateStrings := docstrings.Get("version.update")
	BuildCommandKS(version, runUpdate, updateStrings, os.Stdout)

	return version
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

	saveInstall, _ := ctx.Config.GetString("saveinstall")

	if saveInstall != "" {
		viper.Set(flyctl.ConfigInstaller, saveInstall)
		if err := flyctl.SaveConfig(); err != nil {
			return err
		}
	}

	if ctx.Config.GetBool("full") {
		if ctx.OutputJSON() {
			type flyctlBuild struct {
				Name    string
				Version string
				Commit  string
				Build   string
			}
			ctx.WriteJSON(flyctlBuild{Name: flyname.Name(), Version: flyctl.Version, Commit: flyctl.Commit, Build: flyctl.BuildDate})
		} else {
			fmt.Printf("%s %s %s %s\n", flyname.Name(), flyctl.Version, flyctl.Commit, flyctl.BuildDate)
		}
	} else {
		if ctx.OutputJSON() {
			type flyctlBuild struct {
				Name    string
				Version string
			}
			ctx.WriteJSON(flyctlBuild{Name: flyname.Name(), Version: flyctl.Version})
		} else {
			fmt.Printf("%s %s\n", flyname.Name(), flyctl.Version)
		}
	}
	return nil
}

func runUpdate(ctx *cmdctx.CmdContext) error {
	installerstring := flyctl.CheckForUpdate(true, true) // No skipping, be silent

	if installerstring == "" {
		return fmt.Errorf("no update currently available")
	}

	shellToUse, ok := os.LookupEnv("SHELL")

	if !ok {
		shellToUse = "/bin/bash"
	}

	fmt.Println("Running automatic update [" + installerstring + "]")
	cmd := exec.Command(shellToUse, "-c", installerstring)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	cmd.Run()
	os.Exit(0)
	return nil
}
