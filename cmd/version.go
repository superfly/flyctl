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
		Description: "File for completions - if omitted, defaults to stdout",
	})
	return version
}

func runVersion(ctx *CmdContext) error {
	shellType, _ := ctx.Config.GetString("generate")
	filename, _ := ctx.Config.GetString("file")

	if shellType != "" {
		switch shellType {
		case "bash":
			if filename != "" {
				return GetRootCommand().GenBashCompletionFile(filename)
			} else {
				return GetRootCommand().GenBashCompletion(os.Stdout)
			}
		case "zsh":
			if filename != "" {
				return GetRootCommand().GenZshCompletionFile(filename)
			} else {
				return GetRootCommand().GenZshCompletion(os.Stdout)
			}
		default:
			return fmt.Errorf("unable to generate %s completions", shellType)
		}
	}

	fmt.Printf("flyctl %s %s %s\n", flyctl.Version, flyctl.Commit, flyctl.BuildDate)
	return nil
}
