package cmdctx

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/logrusorgru/aurora"
	"github.com/pkg/errors"
	"github.com/segmentio/textio"
	"github.com/superfly/flyctl/cmd/presenters"
	"github.com/superfly/flyctl/flyctl"
	"github.com/superfly/flyctl/internal/client"
	"github.com/superfly/flyctl/terminal"
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

//Thoughts on how we could illuminate output in flyctl. I'm thinking a set of tags - INFO (plain text),
//DETAIL, TITLE (Bold Plain Text), BEGIN (Green bold with arrow), DONE (Blue with arrow), ERROR (red bold)...

const SINFO = "info"
const SDETAIL = "detail"
const STITLE = "title"
const SBEGIN = "begin"
const SDONE = "done"
const SERROR = "error"

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
func (commandContext *CmdContext) Render(presentable presenters.Presentable) error {
	presenter := &presenters.Presenter{
		Item: presentable,
		Out:  os.Stdout,
		Opts: presenters.Options{
			AsJSON: commandContext.OutputJSON(),
		},
	}

	return presenter.Render()
}

func (commandContext *CmdContext) render(out io.Writer, views ...PresenterOption) error {
	for _, v := range views {
		presenter := &presenters.Presenter{
			Item: v.Presentable,
			Out:  out,
			Opts: presenters.Options{
				Vertical:   v.Vertical,
				HideHeader: v.HideHeader,
				Title:      v.Title,
				AsJSON:     v.AsJSON,
			},
		}

		if err := presenter.Render(); err != nil {
			return err
		}
	}

	return nil
}

// Frender - render a view to a Writer
func (commandContext *CmdContext) Frender(views ...PresenterOption) error {
	// If JSON output wanted, set in all views
	if commandContext.OutputJSON() {
		for i, _ := range views {
			views[i].AsJSON = true
		}
	}

	return commandContext.render(commandContext.Out, views...)
}

// FrenderPrefix - render a view to a Writer
func (commandContext *CmdContext) FrenderPrefix(prefix string, views ...PresenterOption) error {
	// If JSON output wanted, set in all views
	p := textio.NewPrefixWriter(commandContext.Out, "    ")

	if commandContext.OutputJSON() {
		for i, _ := range views {
			views[i].AsJSON = true
		}
	}

	return commandContext.render(p, views...)
}

type JSON struct {
	Source  string
	Status  string
	Message string
}

func (commandContext *CmdContext) StatusLn() {
	outputJSON := commandContext.OutputJSON()

	if outputJSON {
		// Do nothing for JSON
		return
	}

	fmt.Fprintln(commandContext.Out)
}

func (commandContext *CmdContext) Status(source string, status string, args ...interface{}) {
	outputJSON := commandContext.OutputJSON()

	var message strings.Builder

	for i, v := range args {
		message.WriteString(fmt.Sprintf("%s", v))
		if i < len(args)-1 {
			message.WriteString(" ")
		}
	}

	if outputJSON {
		outstruct := JSON{Source: source, Status: status, Message: message.String()}
		outbuf, _ := json.Marshal(outstruct)
		fmt.Fprintln(commandContext.Out, string(outbuf))
		return
	} else {
		fmt.Fprintln(commandContext.Out, statusToEffect(status, message.String()))
	}
}

func statusToEffect(status string, message string) string {
	switch status {
	case SINFO:
		return message
	case SDETAIL:
		return aurora.Faint(message).String()
	case STITLE:
		return aurora.Bold(message).Black().String()
	case SBEGIN:
		return aurora.Green("==> " + message).String()
	case SDONE:
		return aurora.Gray(20, "--> "+message).String()
	case SERROR:
		return aurora.Red("***" + message).String()
	}

	return message
}

func (commandContext *CmdContext) Statusf(source string, status string, format string, args ...interface{}) {
	outputJSON := commandContext.OutputJSON()

	message := fmt.Sprintf(format, args...)

	if outputJSON {
		outbuf, _ := json.Marshal(JSON{Source: source, Status: status, Message: message})
		fmt.Fprintln(commandContext.Out, string(outbuf))
		return
	} else {
		fmt.Fprint(commandContext.Out, statusToEffect(status, message))
	}
}

func (commandContext *CmdContext) WriteJSON(myData interface{}) error {
	outBuf, _ := json.MarshalIndent(myData, "", "    ")
	fmt.Fprintln(commandContext.Out, string(outBuf))
	return nil
}

func (commandContext *CmdContext) OutputJSON() bool {
	return commandContext.GlobalConfig.GetBool(flyctl.ConfigJSONOutput)
}
