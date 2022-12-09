package cmd

import (
	"errors"
	"fmt"

	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/cmd/presenters"
	"github.com/superfly/flyctl/cmdctx"

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
	BuildCommandKS(cmd, runValidateConfig, configValidateStrings, client, requireAppName)

	configEnvStrings := docstrings.Get("config.env")
	BuildCommandKS(cmd, runEnvConfig, configEnvStrings, client, requireSession, requireAppName)

	return cmd
}

func runDisplayConfig(cmdCtx *cmdctx.CmdContext) error {
	ctx := cmdCtx.Command.Context()

	cfg, err := cmdCtx.Client.API().GetConfig(ctx, cmdCtx.AppName)
	if err != nil {
		return err
	}

	// encoder := json.NewEncoder(os.Stdout)
	// encoder.SetIndent("", "  ")
	// encoder.Encode(cfg.Definition)
	cmdCtx.WriteJSON(cfg.Definition)
	return nil
}

func runSaveConfig(cmdCtx *cmdctx.CmdContext) error {
	ctx := cmdCtx.Command.Context()

	configfilename, err := flyctl.ResolveConfigFileFromPath(cmdCtx.WorkingDir)
	if err != nil {
		return err
	}

	if helpers.FileExists(configfilename) {
		cmdCtx.Status("create", cmdctx.SERROR, "An existing configuration file has been found.")
		confirmation := confirm(fmt.Sprintf("Overwrite file '%s'", configfilename))
		if !confirmation {
			return nil
		}
	}

	if cmdCtx.AppConfig == nil {
		cmdCtx.AppConfig = flyctl.NewAppConfig()
	}
	cmdCtx.AppConfig.AppName = cmdCtx.AppName

	serverCfg, err := cmdCtx.Client.API().GetConfig(ctx, cmdCtx.AppName)
	if err != nil {
		return err
	}

	cmdCtx.AppConfig.Definition = serverCfg.Definition

	return writeAppConfig(cmdCtx.ConfigFile, cmdCtx.AppConfig)
}

func runValidateConfig(commandContext *cmdctx.CmdContext) error {
	ctx := commandContext.Command.Context()

	if commandContext.AppConfig == nil {
		return errors.New("App config file not found")
	}

	commandContext.Status("config", cmdctx.STITLE, "Validating", commandContext.ConfigFile)

	// separate query from authenticated app validation (in deploy etc)
	serverCfg, err := client.NewClient("").ValidateConfig(ctx, commandContext.AppName, commandContext.AppConfig.Definition)
	if err != nil {
		return err
	}

	if commandContext.GlobalConfig.GetBool("verbose") {
		commandContext.WriteJSON(serverCfg.Definition)
	}

	if serverCfg.Valid {
		fmt.Println(aurora.Green("✓").String(), "Configuration is valid")
		return nil
	}

	printAppConfigErrors(*serverCfg)

	return errors.New("App configuration is not valid")
}

func runEnvConfig(cmdCtx *cmdctx.CmdContext) error {
	ctx := cmdCtx.Command.Context()

	secrets, err := cmdCtx.Client.API().GetAppSecrets(ctx, cmdCtx.AppName)
	if err != nil {
		return err
	}

	if len(secrets) > 0 {
		err = cmdCtx.Frender(cmdctx.PresenterOption{
			Presentable: &presenters.Secrets{Secrets: secrets},
			Title:       "Secrets",
		})
		if err != nil {
			return err
		}
	}

	cfg, err := cmdCtx.Client.API().GetConfig(ctx, cmdCtx.AppName)
	if err != nil {
		return err
	}

	if cfg.Definition != nil {
		vars, ok := cfg.Definition["env"].(map[string]interface{})
		if !ok {
			return nil
		}

		err = cmdCtx.Frender(cmdctx.PresenterOption{Presentable: &presenters.Environment{
			Envs: vars,
		}, Title: "Environment variables"})

		if err != nil {
			return err
		}
	}
	return nil
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
