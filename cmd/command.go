package cmd

import (
	"fmt"
	"path/filepath"

	"github.com/superfly/flyctl/cmdctx"
	"github.com/superfly/flyctl/docstrings"
	"github.com/superfly/flyctl/flyctl"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/superfly/flyctl/helpers"
	"github.com/superfly/flyctl/internal/buildinfo"
	"github.com/superfly/flyctl/internal/client"
	"github.com/superfly/flyctl/internal/flyerr"
	"github.com/superfly/flyctl/terminal"
)

// RunFn - Run function for commands which takes a command context
type RunFn func(cmdContext *cmdctx.CmdContext) error

// Command - Wrapper for a cobra command
type Command struct {
	*cobra.Command
}

// AddCommand adds subcommands to this command
func (c *Command) AddCommand(commands ...*Command) {
	for _, cmd := range commands {
		c.Command.AddCommand(cmd.Command)
	}
}

func namespace(c *cobra.Command) string {
	parentName := flyctl.NSRoot
	if c.Parent() != nil {
		parentName = c.Parent().Name()
	}
	return parentName + "." + c.Name()
}

// StringFlagOpts - options for string flags
type StringFlagOpts struct {
	Name        string
	Shorthand   string
	Description string
	Default     string
	EnvName     string
	Hidden      bool
}

// BoolFlagOpts - options for boolean flags
type BoolFlagOpts struct {
	Name        string
	Shorthand   string
	Description string
	Default     bool
	EnvName     string
	Hidden      bool
}

// AddStringFlag - Add a string flag to a command
func (c *Command) AddStringFlag(options StringFlagOpts) {
	fullName := namespace(c.Command) + "." + options.Name
	c.Flags().StringP(options.Name, options.Shorthand, options.Default, options.Description)

	flag := c.Flags().Lookup(options.Name)
	flag.Hidden = options.Hidden
	err := viper.BindPFlag(fullName, flag)
	checkErr(err)

	if options.EnvName != "" {
		err := viper.BindEnv(fullName, options.EnvName)
		checkErr(err)
	}
}

// AddBoolFlag - Add a boolean flag for a command
func (c *Command) AddBoolFlag(options BoolFlagOpts) {
	fullName := namespace(c.Command) + "." + options.Name
	c.Flags().BoolP(options.Name, options.Shorthand, options.Default, options.Description)

	flag := c.Flags().Lookup(options.Name)
	flag.Hidden = options.Hidden
	err := viper.BindPFlag(fullName, flag)
	checkErr(err)

	if options.EnvName != "" {
		err := viper.BindEnv(fullName, options.EnvName)
		checkErr(err)
	}
}

// IntFlagOpts - options for integer flags
type IntFlagOpts struct {
	Name        string
	Shorthand   string
	Description string
	Default     int
	EnvName     string
	Hidden      bool
}

// AddIntFlag - Add an integer flag to a command
func (c *Command) AddIntFlag(options IntFlagOpts) {
	fullName := namespace(c.Command) + "." + options.Name
	c.Flags().IntP(options.Name, options.Shorthand, options.Default, options.Description)

	flag := c.Flags().Lookup(options.Name)
	flag.Hidden = options.Hidden
	err := viper.BindPFlag(fullName, flag)
	checkErr(err)

	if options.EnvName != "" {
		err := viper.BindEnv(fullName, options.EnvName)
		checkErr(err)
	}
}

// StringSliceFlagOpts - options a string slice flag
type StringSliceFlagOpts struct {
	Name        string
	Shorthand   string
	Description string
	Default     []string
	EnvName     string
}

// AddStringSliceFlag - add a string slice flag to a command
func (c *Command) AddStringSliceFlag(options StringSliceFlagOpts) {
	fullName := namespace(c.Command) + "." + options.Name

	if options.Shorthand != "" {
		c.Flags().StringSliceP(options.Name, options.Shorthand, options.Default, options.Description)
	} else {
		c.Flags().StringSlice(options.Name, options.Default, options.Description)
	}

	err := viper.BindPFlag(fullName, c.Flags().Lookup(options.Name))
	checkErr(err)

	if options.EnvName != "" {
		err := viper.BindEnv(fullName, options.EnvName)
		checkErr(err)
	}
}

// Initializer - Retains Setup and PreRun functions
type Initializer struct {
	Setup  InitializerFn
	PreRun InitializerFn
}

// Option - A wrapper for an Initializer function that takes a command
type Option func(*Command) Initializer

// InitializerFn - A wrapper for an Initializer function that takes a command context
type InitializerFn func(*cmdctx.CmdContext) error

// BuildCommandKS - A wrapper for BuildCommand which takes the docs.KeyStrings bundle instead of the coder having to manually unwrap it
func BuildCommandKS(parent *Command, fn RunFn, keystrings docstrings.KeyStrings, client *client.Client, options ...Option) *Command {
	return BuildCommand(parent, fn, keystrings.Usage, keystrings.Short, keystrings.Long, client, options...)
}

func BuildCommandCobra(parent *Command, fn RunFn, cmd *cobra.Command, client *client.Client, options ...Option) *Command {
	flycmd := &Command{
		Command: cmd,
	}

	if parent != nil {
		parent.AddCommand(flycmd)
	}

	initializers := []Initializer{}

	for _, o := range options {
		if i := o(flycmd); i.Setup != nil || i.PreRun != nil {
			initializers = append(initializers, i)
		}
	}

	if fn != nil {
		flycmd.RunE = func(cmd *cobra.Command, args []string) error {
			ctx, err := cmdctx.NewCmdContext(client, namespace(cmd), cmd, args)
			if err != nil {
				return err
			}

			for _, init := range initializers {
				if init.Setup != nil {
					if err := init.Setup(ctx); err != nil {
						return err
					}
				}
			}

			terminal.Debugf("Working Directory: %s\n", ctx.WorkingDir)
			terminal.Debugf("App Config File: %s\n", ctx.ConfigFile)

			for _, init := range initializers {
				if init.PreRun != nil {
					if err := init.PreRun(ctx); err != nil {
						return err
					}
				}
			}

			return fn(ctx)
		}
	}

	return flycmd
}

// BuildCommand - builds a functioning Command using all the initializers
func BuildCommand(parent *Command, fn RunFn, usageText string, shortHelpText string, longHelpText string, client *client.Client, options ...Option) *Command {
	return BuildCommandCobra(parent, fn, &cobra.Command{
		Use:   usageText,
		Short: shortHelpText,
		Long:  longHelpText,
	}, client, options...)

}

const defaultConfigFilePath = "./fly.toml"

func requireSession(cmd *Command) Initializer {
	return Initializer{
		PreRun: func(ctx *cmdctx.CmdContext) error {
			if !ctx.Client.Authenticated() {
				return client.ErrNoAuthToken
			}
			return nil
		},
	}
}

func addAppConfigFlags(cmd *Command) {
	cmd.AddStringFlag(StringFlagOpts{
		Name:        "app",
		Shorthand:   "a",
		Description: "App name to operate on",
		EnvName:     "FLY_APP",
	})
	cmd.AddStringFlag(StringFlagOpts{
		Name:        "config",
		Shorthand:   "c",
		Description: "Path to an app config file or directory containing one",
		Default:     defaultConfigFilePath,
		EnvName:     "FLY_APP_CONFIG",
	})
}

func setupAppName(ctx *cmdctx.CmdContext) error {
	// resolve the config file path
	configPath := ctx.Config.GetString("config")
	if configPath == "" {
		configPath = defaultConfigFilePath
	}
	if !filepath.IsAbs(configPath) {
		absConfigPath, err := filepath.Abs(filepath.Join(ctx.WorkingDir, configPath))
		if err != nil {
			return err
		}
		configPath = absConfigPath
	}
	resolvedPath, err := flyctl.ResolveConfigFileFromPath(configPath)
	if err != nil {
		return err
	}
	ctx.ConfigFile = resolvedPath

	// load the config file if it exists
	if helpers.FileExists(ctx.ConfigFile) {
		terminal.Debug("Loading app config from", ctx.ConfigFile)
		appConfig, err := flyctl.LoadAppConfig(ctx.ConfigFile)
		if err != nil {
			return err
		}
		ctx.AppConfig = appConfig
	} else {
		ctx.AppConfig = flyctl.NewAppConfig()
	}

	// set the app name if provided
	appName := ctx.Config.GetString("app")
	if appName != "" {
		ctx.AppName = appName
	} else if ctx.AppConfig != nil {
		ctx.AppName = ctx.AppConfig.AppName
	}

	return nil
}

func optionalAppName(cmd *Command) Initializer {
	addAppConfigFlags(cmd)
	return Initializer{
		Setup: setupAppName,
	}
}

func requireAppName(cmd *Command) Initializer {
	// TODO: Add Flags to docStrings

	addAppConfigFlags(cmd)

	return Initializer{
		Setup: setupAppName,
		PreRun: func(ctx *cmdctx.CmdContext) error {
			if ctx.AppName == "" {
				return fmt.Errorf("We couldn't find a fly.toml nor an app specified by the -a flag. If you want to launch a new app, use '" + buildinfo.Name() + " launch'")
			}

			if ctx.AppConfig == nil {
				return nil
			}

			if ctx.AppConfig.AppName != "" && ctx.AppConfig.AppName != ctx.AppName {
				terminal.Warnf("app flag '%s' does not match app name in config file '%s'\n", ctx.AppName, ctx.AppConfig.AppName)

				if !confirm(fmt.Sprintf("Continue using '%s'", ctx.AppName)) {
					return flyerr.ErrAbort
				}
			}

			return nil
		},
	}
}

func requireAppNameAsArg(cmd *Command) Initializer {
	cmd.AddStringFlag(StringFlagOpts{
		Name:        "app",
		Shorthand:   "a",
		Description: "App name to operate on",
		EnvName:     "FLY_APP",
	})

	cmd.AddStringFlag(StringFlagOpts{
		Name:        "config",
		Shorthand:   "c",
		Description: "Path to an app config file or directory containing one",
		Default:     defaultConfigFilePath,
		EnvName:     "FLY_APP_CONFIG",
	})

	return Initializer{
		Setup: func(ctx *cmdctx.CmdContext) error {
			// resolve the config file path
			configPath := ctx.Config.GetString("config")
			if configPath == "" {
				configPath = defaultConfigFilePath
			}
			if !filepath.IsAbs(configPath) {
				absConfigPath, err := filepath.Abs(filepath.Join(ctx.WorkingDir, configPath))
				if err != nil {
					return err
				}
				configPath = absConfigPath
			}
			resolvedPath, err := flyctl.ResolveConfigFileFromPath(configPath)
			if err != nil {
				return err
			}
			ctx.ConfigFile = resolvedPath

			// load the config file if it exists
			if helpers.FileExists(ctx.ConfigFile) {
				terminal.Debug("Loading app config from", ctx.ConfigFile)
				appConfig, err := flyctl.LoadAppConfig(ctx.ConfigFile)
				if err != nil {
					return err
				}
				ctx.AppConfig = appConfig
			} else {
				ctx.AppConfig = flyctl.NewAppConfig()
			}

			// set the app name if provided
			appName := ctx.Config.GetString("app")
			if appName != "" {
				ctx.AppName = appName
			} else if ctx.AppConfig != nil {
				ctx.AppName = ctx.AppConfig.AppName
			}

			return nil
		},
		PreRun: func(ctx *cmdctx.CmdContext) error {
			if len(ctx.Args) > 0 {
				ctx.AppName = ctx.Args[0]
			}

			if ctx.AppName == "" {
				return fmt.Errorf("No app specified")
			}

			if ctx.AppConfig == nil {
				return nil
			}

			if ctx.AppConfig.AppName != "" && ctx.AppConfig.AppName != ctx.AppName {
				terminal.Warnf("app flag '%s' does not match app name in config file '%s'\n", ctx.AppName, ctx.AppConfig.AppName)

				if !confirm(fmt.Sprintf("Continue using '%s'", ctx.AppName)) {
					return flyerr.ErrAbort
				}
			}

			return nil
		},
	}
}
