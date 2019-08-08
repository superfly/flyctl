package cmd

import (
	"fmt"
	"io"
	"os"

	"github.com/logrusorgru/aurora"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/cmd/presenters"
	"github.com/superfly/flyctl/flyctl"
)

type CmdRunFn func(*CmdContext) error

type Command struct {
	*cobra.Command
	requireSession bool
	requireAppName bool
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

func (c *Command) AddStringFlag(options StringFlagOpts) {
	fullName := namespace(c.Command) + "." + options.Name
	c.Flags().StringP(options.Name, options.Shorthand, options.Default, options.Description)

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

func newCmdContext(ns string, out io.Writer, args []string, initClient bool, initApp bool) (*CmdContext, error) {
	ctx := &CmdContext{
		NS:           ns,
		Config:       flyctl.ConfigNS(ns),
		GlobalConfig: flyctl.FlyConfig,
		Out:          out,
		Args:         args,
	}

	if initClient {
		client, err := api.NewClient()
		if err != nil {
			return nil, err
		}
		ctx.FlyClient = client
	}

	return ctx, nil
}

type CmdOption func(*Command) InitializerFn
type InitializerFn func(*CmdContext) error

func BuildCommand(parent *Command, fn CmdRunFn, useText, helpText string, out io.Writer, initClient bool, options ...CmdOption) *Command {
	flycmd := &Command{
		requireSession: true,
		Command: &cobra.Command{
			Use:   useText,
			Short: helpText,
			Long:  helpText,
		},
	}

	if parent != nil {
		parent.AddCommand(flycmd)
	}

	initializers := []InitializerFn{}

	for _, o := range options {
		if initFn := o(flycmd); initFn != nil {
			initializers = append(initializers, initFn)
		}
	}

	if fn != nil {
		flycmd.Run = func(cmd *cobra.Command, args []string) {
			ctx, err := newCmdContext(namespace(cmd), out, args, flycmd.requireSession, flycmd.requireAppName)
			checkErr(err)

			for _, init := range initializers {
				err = init(ctx)
				checkErr(err)
			}

			err = fn(ctx)
			checkErr(err)
		}
	}

	return flycmd
}

func requireAppName(cmd *Command) InitializerFn {
	cmd.requireAppName = true
	cmd.AddStringFlag(StringFlagOpts{
		Name:        "app",
		Shorthand:   "a",
		Description: "app to operate on",
	})

	return func(ctx *CmdContext) error {
		if p, err := flyctl.LoadProject("."); err == nil {
			ctx.Project = p
		}

		if ctx.AppName() == "" {
			return fmt.Errorf("No app specified")
		}
		return nil
	}
}

func requireProject(cmd *Command) InitializerFn {
	return func(ctx *CmdContext) error {
		p, err := flyctl.LoadProject(".")
		if err != nil {
			return err
		}
		ctx.Project = p

		return nil
	}
}

func requireSession(val bool) func(*Command) {
	return func(cmd *Command) {
		cmd.requireSession = val
	}
}

func checkErr(err error) {
	if err == nil {
		return
	}

	fmt.Println(aurora.Red("Error"), err)

	os.Exit(1)
}
