package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"

	"github.com/spf13/viper"
	"github.com/superfly/flyctl/cmdctx"
	"github.com/superfly/flyctl/flyctl"
	"github.com/superfly/flyctl/flyname"

	"github.com/superfly/flyctl/docstrings"
)

func newVersionCommand() *Command {
	versionStrings := docstrings.Get("version")
	version := BuildCommandKS(nil, runVersion, versionStrings, os.Stdout)
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

func runUpdate(ctx *cmdctx.CmdContext) error {
	installerstring := flyctl.CheckForUpdate(true, true) // No skipping, be silent

	if installerstring == "" {
		return fmt.Errorf("no update currently available")
	}

	shellToUse, ok := os.LookupEnv("SHELL")
	switchToUse := "-c"

	if !ok {
		if runtime.GOOS == "windows" {
			shellToUse = "powershell.exe"
			switchToUse = "-Command"
		} else {
			shellToUse = "/bin/bash"
		}
	}
	fmt.Println(shellToUse, switchToUse)

	fmt.Println("Running automatic update [" + installerstring + "]")
	cmd := exec.Command(shellToUse, switchToUse, installerstring)

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	err := cmd.Run()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	os.Exit(0)
	return nil
}
