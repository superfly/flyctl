package cmd

import (
	"fmt"
	"github.com/superfly/flyctl/docstrings"
	"os"

	"github.com/superfly/flyctl/flyctl"
)

func newVersionCommand() *Command {
	versionStrings := docstrings.Get("version")
	version := BuildCommand(nil, runVersion, versionStrings.Usage, versionStrings.Short, versionStrings.Long, false, os.Stdout)
	version.AddStringFlag(StringFlagOpts{
		Name:        "generate",
		Description: "Generate completions for this version (parameter bash/zsh)",
	})
	version.AddStringFlag(StringFlagOpts{
		Name:        "file",
		Description: "File for completions",
	})
	return version
}

func runVersion(ctx *CmdContext) error {
	val, _ := ctx.Config.GetString("generate")
	fval, _ := ctx.Config.GetString("file")

	if val != "" && fval != "" {
		if val == "bash" {
			return GetRootCommand().GenBashCompletionFile(fval)
		} else if val == "zsh" {
			return GetRootCommand().GenZshCompletionFile(fval)
		}
		return fmt.Errorf("unable to generate %s completions", val)
	}

	fmt.Printf("flyctl %s %s %s\n", flyctl.Version, flyctl.Commit, flyctl.BuildDate)
	return nil
}
