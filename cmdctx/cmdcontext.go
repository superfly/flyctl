package cmdctx

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/logrusorgru/aurora"
	"github.com/pkg/errors"
	"github.com/segmentio/textio"
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/cmd/presenters"
	"github.com/superfly/flyctl/flyctl"
	"github.com/superfly/flyctl/internal/client"
	"github.com/superfly/flyctl/iostreams"
)

// CmdContext - context passed to commands being run
type CmdContext struct {
	IO            *iostreams.IOStreams
	Client        *client.Client
	Config        flyctl.Config
	GlobalConfig  flyctl.Config
	NS            string
	Args          []string
	Command       *cobra.Command
	Out           io.Writer
	WorkingDir    string
	ConfigFile    string
	AppName       string
	AppConfig     *flyctl.AppConfig
	MachineConfig *api.MachineConfig
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
const SWARN = "warning"
const SDETAIL = "detail"
const STITLE = "title"
const SBEGIN = "begin"
const SDONE = "done"
const SERROR = "error"

func NewCmdContext(flyctlClient *client.Client, ns string, cmd *cobra.Command, args []string) (*CmdContext, error) {
	ctx := &CmdContext{
		IO:           flyctlClient.IO,
		Client:       flyctlClient,
		NS:           ns,
		Config:       flyctl.ConfigNS(ns),
		GlobalConfig: flyctl.FlyConfig,
		Args:         args,
		Command:      cmd,
		Out:          flyctlClient.IO.Out,
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
		for i := range views {
			views[i].AsJSON = true
		}
	}

	return commandContext.render(commandContext.IO.Out, views...)
}

// FrenderPrefix - render a view to a Writer
func (commandContext *CmdContext) FrenderPrefix(prefix string, views ...PresenterOption) error {
	// If JSON output wanted, set in all views
	p := textio.NewPrefixWriter(commandContext.IO.Out, "    ")

	if commandContext.OutputJSON() {
		for i := range views {
			views[i].AsJSON = true
		}
	}

	return commandContext.render(p, views...)
}

type JSON struct {
	TS      string
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

	fmt.Fprintln(commandContext.IO.Out)
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
		outstruct := JSON{TS: time.Now().Format(time.RFC3339),
			Source:  source,
			Status:  status,
			Message: message.String()}
		outbuf, _ := json.Marshal(outstruct)
		fmt.Fprintln(commandContext.IO.Out, string(outbuf))
		return
	} else {
		fmt.Fprintln(commandContext.IO.Out, statusToEffect(status, message.String()))
	}
}

func statusToEffect(status string, message string) string {
	switch status {
	case SINFO:
		return message
	case SWARN:
		return aurora.Yellow(message).String()
	case SDETAIL:
		return aurora.Faint(message).String()
	case STITLE:
		return aurora.Bold(message).String()
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
		outbuf, _ := json.Marshal(JSON{TS: time.Now().Format(time.RFC3339),
			Source:  source,
			Status:  status,
			Message: message})
		fmt.Fprintln(commandContext.IO.Out, string(outbuf))
		return
	} else {
		fmt.Fprint(commandContext.IO.Out, statusToEffect(status, message))
	}
}

func (commandContext *CmdContext) WriteJSON(myData interface{}) {
	outBuf, _ := json.MarshalIndent(myData, "", "    ")
	fmt.Fprintln(commandContext.IO.Out, string(outBuf))
}

func (commandContext *CmdContext) OutputJSON() bool {
	return commandContext.GlobalConfig.GetBool(flyctl.ConfigJSONOutput)
}
