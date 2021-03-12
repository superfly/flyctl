package cmd

import (
	"errors"
	"fmt"

	"github.com/superfly/flyctl/cmdctx"
	"github.com/superfly/flyctl/internal/client"

	"github.com/superfly/flyctl/docstrings"

	"github.com/logrusorgru/aurora"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/flyctl"
	"github.com/superfly/flyctl/helpers"
)

func newConfigCommand(client *client.Client) *Command {

	configStrings := docstrings.Get("config")

	cmd := BuildCommandKS(nil, nil, configStrings, client, requireSession, requireAppName)

	configDisplayStrings := docstrings.Get("config.display")
	BuildCommandKS(cmd, runDisplayConfig, configDisplayStrings, client, requireSession, requireAppName)

	configSaveStrings := docstrings.Get("config.save")
	BuildCommandKS(cmd, runSaveConfig, configSaveStrings, client, requireSession, requireAppName)

	configValidateStrings := docstrings.Get("config.validate")
	BuildCommandKS(cmd, runValidateConfig, configValidateStrings, client, requireSession, requireAppName)

	return cmd
}

func runDisplayConfig(ctx *cmdctx.CmdContext) error {
	cfg, err := ctx.Client.API().GetConfig(ctx.AppName)
	if err != nil {
		return err
	}

	//encoder := json.NewEncoder(os.Stdout)
	//encoder.SetIndent("", "  ")
	//encoder.Encode(cfg.Definition)
	ctx.WriteJSON(cfg.Definition)
	return nil
}

func runSaveConfig(ctx *cmdctx.CmdContext) error {
	configfilename, err := flyctl.ResolveConfigFileFromPath(ctx.WorkingDir)

	if err != nil {
		return err
	}

	if helpers.FileExists(configfilename) {
		ctx.Status("create", cmdctx.SERROR, "An existing configuration file has been found.")
		confirmation := confirm(fmt.Sprintf("Overwrite file '%s'", configfilename))
		if !confirmation {
			return nil
		}
	}

	if ctx.AppConfig == nil {
		ctx.AppConfig = flyctl.NewAppConfig()
	}
	ctx.AppConfig.AppName = ctx.AppName

	serverCfg, err := ctx.Client.API().GetConfig(ctx.AppName)
	if err != nil {
		return err
	}

	ctx.AppConfig.Definition = serverCfg.Definition

	return writeAppConfig(ctx.ConfigFile, ctx.AppConfig)
}

func runValidateConfig(commandContext *cmdctx.CmdContext) error {
	if commandContext.AppConfig == nil {
		return errors.New("App config file not found")
	}

	commandContext.Status("config", cmdctx.STITLE, "Validating", commandContext.ConfigFile)

	serverCfg, err := commandContext.Client.API().ParseConfig(commandContext.AppName, commandContext.AppConfig.Definition)
	if err != nil {
		return err
	}

	if serverCfg.Valid {
		fmt.Println(aurora.Green("✓").String(), "Configuration is valid")
		return nil
	}

	printAppConfigErrors(*serverCfg)

	return errors.New("App configuration is not valid")
}

func printAppConfigErrors(cfg api.AppConfig) {
	fmt.Println()
	for _, error := range cfg.Errors {
		fmt.Println("   ", aurora.Red("✘").String(), error)
	}
	fmt.Println()
}

func writeAppConfig(path string, appConfig *flyctl.AppConfig) error {

	if err := appConfig.WriteToFile(path); err != nil {
		return err
	}

	fmt.Println("Wrote config file", helpers.PathRelativeToCWD(path))

	return nil
}
