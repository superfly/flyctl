package cmd

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/signal"
	"path"
	"path/filepath"
	"syscall"

	"github.com/superfly/flyctl/cmdctx"
	"github.com/superfly/flyctl/docstrings"
	"github.com/superfly/flyctl/flyctl"
	"github.com/superfly/flyctl/flyname"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/superfly/flyctl/helpers"
	"github.com/superfly/flyctl/internal/client"
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

	err := viper.BindPFlag(fullName, c.Flags().Lookup(options.Name))
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

// BuildCommand - builds a functioning Command using all the initializers
func BuildCommand(parent *Command, fn RunFn, usageText string, shortHelpText string, longHelpText string, client *client.Client, options ...Option) *Command {
	flycmd := &Command{
		Command: &cobra.Command{
			Use:   usageText,
			Short: shortHelpText,
			Long:  longHelpText,
		},
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
		flycmd.Run = func(cmd *cobra.Command, args []string) {
			ctx, err := cmdctx.NewCmdContext(client, namespace(cmd), args)
			checkErr(err)

			for _, init := range initializers {
				if init.Setup != nil {
					checkErr(init.Setup(ctx))
				}
			}

			terminal.Debugf("Working Directory: %s\n", ctx.WorkingDir)
			terminal.Debugf("App Config File: %s\n", ctx.ConfigFile)

			for _, init := range initializers {
				if init.PreRun != nil {
					checkErr(init.PreRun(ctx))
				}
			}

			err = fn(ctx)
			checkErr(err)
		}
	}

	return flycmd
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

func requireAppName(cmd *Command) Initializer {
	// TODO: Add Flags to docStrings

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
			if ctx.AppName == "" {
				return fmt.Errorf("No app specified. Specify an app or create an app with '" + flyname.Name() + " init'")
			}

			if ctx.AppConfig == nil {
				return nil
			}

			if ctx.AppConfig.AppName != "" && ctx.AppConfig.AppName != ctx.AppName {
				// Quick check for a fly.alias
				present, err := checkAliasFile(ctx.AppName)
				if err != nil {
					return err
				}
				if present {
					return nil
				}

				terminal.Warnf("app flag '%s' does not match app name in config file '%s'\n", ctx.AppName, ctx.AppConfig.AppName)

				if !confirm(fmt.Sprintf("Continue using '%s'", ctx.AppName)) {
					return ErrAbort
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
				// Quick check for a fly.alias
				present, err := checkAliasFile(ctx.AppName)
				if err != nil {
					return err
				}
				if present {
					return nil
				}

				terminal.Warnf("app flag '%s' does not match app name in config file '%s'\n", ctx.AppName, ctx.AppConfig.AppName)

				if !confirm(fmt.Sprintf("Continue using '%s'", ctx.AppName)) {
					return ErrAbort
				}
			}

			return nil
		},
	}
}

func checkAliasFile(appname string) (present bool, err error) {
	if helpers.FileExists("fly.alias") {
		file, err := os.Open("fly.alias")
		if err != nil {
			return false, err
		}
		defer file.Close()

		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			if scanner.Text() == appname {
				return true, nil
			}
		}

		if err := scanner.Err(); err != nil {
			return false, err
		}
	}
	return false, nil
}
func workingDirectoryFromArg(index int) func(*Command) Initializer {
	return func(cmd *Command) Initializer {
		return Initializer{
			Setup: func(ctx *cmdctx.CmdContext) error {
				if len(ctx.Args) <= index {
					return nil
					// return fmt.Errorf("cannot resolve working directory from arg %d, not enough args", index)
				}
				wd := ctx.Args[index]

				if !path.IsAbs(wd) {
					wd = path.Join(ctx.WorkingDir, wd)
				}

				abs, err := filepath.Abs(wd)
				if err != nil {
					return err
				}
				ctx.WorkingDir = abs

				if !helpers.DirectoryExists(ctx.WorkingDir) {
					return fmt.Errorf("working directory '%s' not found", ctx.WorkingDir)
				}

				return nil
			},
		}
	}
}

func createCancellableContext() context.Context {
	signals := make(chan os.Signal)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		<-signals
		cancel()
	}()

	return ctx
}
