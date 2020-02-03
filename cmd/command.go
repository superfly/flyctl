package cmd

import (
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"

	"github.com/logrusorgru/aurora"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/cmd/presenters"
	"github.com/superfly/flyctl/flyctl"
	"github.com/superfly/flyctl/helpers"
	"github.com/superfly/flyctl/terminal"
)

type CmdRunFn func(*CmdContext) error

type Command struct {
	*cobra.Command
	requireSession bool
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

type StringFlagOpts struct {
	Name        string
	Shorthand   string
	Description string
	Default     string
	EnvName     string
}

type BoolFlagOpts struct {
	Name        string
	Shorthand   string
	Description string
	Default     bool
	EnvName     string
}

func (c *Command) AddStringFlag(options StringFlagOpts) {
	fullName := namespace(c.Command) + "." + options.Name
	c.Flags().StringP(options.Name, options.Shorthand, options.Default, options.Description)

	viper.BindPFlag(fullName, c.Flags().Lookup(options.Name))

	if options.EnvName != "" {
		viper.BindEnv(fullName, options.EnvName)
	}
}

func (c *Command) AddBoolFlag(options BoolFlagOpts) {
	fullName := namespace(c.Command) + "." + options.Name
	c.Flags().BoolP(options.Name, options.Shorthand, options.Default, options.Description)

	viper.BindPFlag(fullName, c.Flags().Lookup(options.Name))

	if options.EnvName != "" {
		viper.BindEnv(fullName, options.EnvName)
	}
}

type StringSliceFlagOpts struct {
	Name        string
	Shorthand   string
	Description string
	Default     []string
	EnvName     string
}

func (c *Command) AddStringSliceFlag(options StringSliceFlagOpts) {
	fullName := namespace(c.Command) + "." + options.Name

	if options.Shorthand != "" {
		c.Flags().StringSliceP(options.Name, options.Shorthand, options.Default, options.Description)
	} else {
		c.Flags().StringSlice(options.Name, options.Default, options.Description)
	}

	viper.BindPFlag(fullName, c.Flags().Lookup(options.Name))

	if options.EnvName != "" {
		viper.BindEnv(fullName, options.EnvName)
	}
}

type CmdContext struct {
	Config       flyctl.Config
	GlobalConfig flyctl.Config
	NS           string
	Args         []string
	Out          io.Writer
	FlyClient    *api.Client
	Terminal     *terminal.Terminal
	WorkingDir   string
	ConfigFile   string

	AppName   string
	AppConfig *flyctl.AppConfig
}

func (ctx *CmdContext) InitApiClient() error {
	client, err := api.NewClient(viper.GetString(flyctl.ConfigAPIToken), flyctl.Version)
	if err != nil {
		return err
	}
	ctx.FlyClient = client
	return nil
}

func (ctx *CmdContext) Render(presentable presenters.Presentable) error {
	presenter := &presenters.Presenter{
		Item: presentable,
		Out:  os.Stdout,
	}

	return presenter.Render()
}

func (ctx *CmdContext) RenderEx(presentable presenters.Presentable, options presenters.Options) error {
	presenter := &presenters.Presenter{
		Item: presentable,
		Out:  os.Stdout,
		Opts: options,
	}

	return presenter.Render()
}

type PresenterOption struct {
	Presentable presenters.Presentable
	Vertical    bool
	HideHeader  bool
	Title       string
}

func (ctx *CmdContext) render(out io.Writer, views ...PresenterOption) error {
	for _, v := range views {
		presenter := &presenters.Presenter{
			Item: v.Presentable,
			Out:  out,
			Opts: presenters.Options{
				Vertical:   v.Vertical,
				HideHeader: v.HideHeader,
			},
		}

		if v.Title != "" {
			fmt.Fprintln(out, aurora.Bold(v.Title))
		}

		if err := presenter.Render(); err != nil {
			return err
		}
	}

	return nil
}

func (ctx *CmdContext) RenderView(views ...PresenterOption) (err error) {
	return ctx.render(ctx.Terminal, views...)
}

func (ctx *CmdContext) RenderViewW(w io.Writer, views ...PresenterOption) error {
	return ctx.render(w, views...)
}

func newCmdContext(ns string, out io.Writer, args []string, initClient bool) (*CmdContext, error) {
	ctx := &CmdContext{
		NS:           ns,
		Config:       flyctl.ConfigNS(ns),
		GlobalConfig: flyctl.FlyConfig,
		Out:          out,
		Args:         args,
		Terminal:     terminal.NewTerminal(out),
	}

	cwd, err := os.Getwd()
	if err != nil {
		return nil, errors.Wrap(err, "Error resolving working directory")
	}
	ctx.WorkingDir = cwd

	if initClient {
		if err := ctx.InitApiClient(); err != nil {
			return nil, err
		}
	}

	return ctx, nil
}

type Initializer struct {
	Setup  InitializerFn
	PreRun InitializerFn
}
type CmdOption func(*Command) Initializer
type InitializerFn func(*CmdContext) error

func BuildCommand(parent *Command, fn CmdRunFn, usageText string, shortHelpText string, longHelpText string, initClient bool, out io.Writer, options ...CmdOption) *Command {
	flycmd := &Command{
		requireSession: initClient,
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
			ctx, err := newCmdContext(namespace(cmd), out, args, flycmd.requireSession)
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
		Setup: func(ctx *CmdContext) error {
			// resolve the config file path
			configPath, _ := ctx.Config.GetString("config")
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
			appName, _ := ctx.Config.GetString("app")
			if appName != "" {
				ctx.AppName = appName
			} else if ctx.AppConfig != nil {
				ctx.AppName = ctx.AppConfig.AppName
			}

			return nil
		},
		PreRun: func(ctx *CmdContext) error {
			if ctx.AppName == "" {
				return fmt.Errorf("No app specified")
			}

			if ctx.AppConfig == nil {
				return nil
			}

			if ctx.AppConfig.AppName != "" && ctx.AppConfig.AppName != ctx.AppName {
				terminal.Warnf("app flag '%s' does not match app name in config file '%s'\n", ctx.AppName, ctx.AppConfig.AppName)

				if !confirm(fmt.Sprintf("Continue using '%s'", ctx.AppName)) {
					return ErrAbort
				}
			}

			return nil
		},
	}
}

func workingDirectoryFromArg(index int) func(*Command) Initializer {
	return func(cmd *Command) Initializer {
		return Initializer{
			Setup: func(ctx *CmdContext) error {
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

func requireSession(val bool) func(*Command) {
	return func(cmd *Command) {
		cmd.requireSession = val
	}
}
