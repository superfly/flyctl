package cmdctx

import (
	"fmt"
	"github.com/logrusorgru/aurora"
	"github.com/pkg/errors"
	"github.com/superfly/flyctl/cmd/presenters"
	"github.com/superfly/flyctl/flyctl"
	"github.com/superfly/flyctl/internal/client"
	"github.com/superfly/flyctl/terminal"
	"io"
	"os"
)

// CmdContext - context passed to commands being run
type CmdContext struct {
	Client       *client.Client
	Config       flyctl.Config
	GlobalConfig flyctl.Config
	NS           string
	Args         []string
	Out          io.Writer
	Terminal     *terminal.Terminal
	WorkingDir   string
	ConfigFile   string
	AppName      string
	AppConfig    *flyctl.AppConfig
}

// PresenterOption - options for RenderEx, RenderView, render etc...
type PresenterOption struct {
	Presentable presenters.Presentable
	AsJSON      bool
	Vertical    bool
	HideHeader  bool
	Title       string
}

// Render - Render a presentable structure via the context
func (ctx *CmdContext) Render(presentable presenters.Presentable) error {
	presenter := &presenters.Presenter{
		Item: presentable,
		Out:  os.Stdout,
	}

	return presenter.Render()
}

// RenderEx - Render a presentable structure via the context with additional options
func (ctx *CmdContext) RenderEx(presentable presenters.Presentable, options presenters.Options) error {
	presenter := &presenters.Presenter{
		Item: presentable,
		Out:  os.Stdout,
		Opts: options,
	}

	return presenter.Render()
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

// RenderView - render a view through the context to the terminal
func (ctx *CmdContext) RenderView(views ...PresenterOption) (err error) {
	return ctx.render(ctx.Terminal, views...)
}

// RenderViewW - render a view to a Writer
func (ctx *CmdContext) RenderViewW(w io.Writer, views ...PresenterOption) error {
	return ctx.render(w, views...)
}

func NewCmdContext(flyctlClient *client.Client, ns string, out io.Writer, args []string) (*CmdContext, error) {
	ctx := &CmdContext{
		Client:       flyctlClient,
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

	return ctx, nil
}

// Render - Render a presentable structure via the context
func Render(presentable presenters.Presentable) error {
	presenter := &presenters.Presenter{
		Item: presentable,
		Out:  os.Stdout,
	}

	return presenter.Render()
}

// Frender - render a view to a Writer
func (ctx *CmdContext) Frender(w io.Writer, views ...PresenterOption) error {
	// If JSON output wanted, set in all views
	if ctx.GlobalConfig.GetBool(flyctl.ConfigJSONOutput) {
		for i, _ := range views {
			views[i].AsJSON = true
		}
	}

	return ctx.render(w, views...)
}
