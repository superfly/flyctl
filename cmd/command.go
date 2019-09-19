package cmd

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/cmd/presenters"
	"github.com/superfly/flyctl/flyctl"
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
}

type BoolFlagOpts struct {
	Name        string
	Shorthand   string
	Description string
	Default     bool
}

func (c *Command) AddStringFlag(options StringFlagOpts) {
	fullName := namespace(c.Command) + "." + options.Name
	c.Flags().StringP(options.Name, options.Shorthand, options.Default, options.Description)

	viper.BindPFlag(fullName, c.Flags().Lookup(options.Name))
}

func (c *Command) AddBoolFlag(options BoolFlagOpts) {
	fullName := namespace(c.Command) + "." + options.Name
	c.Flags().BoolP(options.Name, options.Shorthand, options.Default, options.Description)

	viper.BindPFlag(fullName, c.Flags().Lookup(options.Name))
}

type CmdContext struct {
	Config       flyctl.Config
	GlobalConfig flyctl.Config
	NS           string
	Args         []string
	Out          io.Writer
	FlyClient    *api.Client
	Project      *flyctl.Project
}

func (ctx *CmdContext) InitApiClient() error {
	client, err := api.NewClient(viper.GetString(flyctl.ConfigAPIToken))
	if err != nil {
		return err
	}
	ctx.FlyClient = client
	return nil
}

func (ctx *CmdContext) AppName() string {
	if name, _ := ctx.Config.GetString(flyctl.ConfigAppName); name != "" {
		return name
	}

	if ctx.Project != nil {
		return ctx.Project.AppName()
	}

	return ""
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

func newCmdContext(ns string, out io.Writer, args []string, initClient bool) (*CmdContext, error) {
	ctx := &CmdContext{
		NS:           ns,
		Config:       flyctl.ConfigNS(ns),
		GlobalConfig: flyctl.FlyConfig,
		Out:          out,
		Args:         args,
	}

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

func BuildCommand(parent *Command, fn CmdRunFn, useText, helpText string, out io.Writer, initClient bool, options ...CmdOption) *Command {
	flycmd := &Command{
		requireSession: initClient,
		Command: &cobra.Command{
			Use:   useText,
			Short: helpText,
			Long:  helpText,
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

func requireAppName(cmd *Command) Initializer {
	cmd.AddStringFlag(StringFlagOpts{
		Name:        "app",
		Shorthand:   "a",
		Description: "app to operate on",
	})

	return Initializer{
		Setup: func(ctx *CmdContext) error {
			if p, err := flyctl.LoadProject("./fly.toml"); err == nil {
				ctx.Project = p
			}

			return nil
		},
		PreRun: func(ctx *CmdContext) error {
			if ctx.AppName() == "" {
				return fmt.Errorf("No app specified")
			}
			return nil
		},
	}
}

func loadProjectFromPathInFirstArg(cmd *Command) Initializer {
	return Initializer{
		Setup: func(ctx *CmdContext) error {
			cfgPath := "."
			if len(ctx.Args) > 0 {
				cfgPath = ctx.Args[0]
			}

			p, err := flyctl.LoadProject(cfgPath)
			if err != nil {
				return err
			}
			ctx.Project = p

			return nil
		},
		PreRun: func(ctx *CmdContext) error {
			if ctx.Project == nil {
				return nil
			}

			if ctx.Project.ConfigFileLoaded() && ctx.Project.AppName() != "" && ctx.Project.AppName() != ctx.AppName() {
				terminal.Warnf("app flag '%s' does not match app name in config file '%s'\n", ctx.AppName(), ctx.Project.AppName())

				if !confirm(fmt.Sprintf("Continue deploying to '%s'", ctx.AppName())) {
					return ErrAbort
				}
			}

			return nil
		},
	}
}

func requireSession(val bool) func(*Command) {
	return func(cmd *Command) {
		cmd.requireSession = val
	}
}
