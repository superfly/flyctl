package output

import (
	"encoding/json"
	"fmt"
	"github.com/superfly/flyctl/cmdctx"
	"github.com/superfly/flyctl/flyctl"
)

type Manager struct {
	Context *cmdctx.CmdContext
}

type JSON struct {
	Source  string
	Message string
}

func NewManager(ctx *cmdctx.CmdContext) *Manager {
	return &Manager{Context: ctx}
}

func (om *Manager) StatusOut(source string, args ...interface{}) {
	outputJSON := om.Context.GlobalConfig.GetBool(flyctl.ConfigJSONOutput)

	message := ""
	for _, v := range args {
		message = fmt.Sprintf("%s %v", message, v)
	}

	if outputJSON {
		outstruct := JSON{Source: source, Message: message}
		outbuf, _ := json.Marshal(outstruct)
		fmt.Fprintln(om.Context.Out, string(outbuf))
		return
	} else {
		fmt.Fprintln(om.Context.Out, message)
	}
}

func (om *Manager) StatusOutf(source string, format string, args ...interface{}) {
	outputJSON := om.Context.GlobalConfig.GetBool(flyctl.ConfigJSONOutput)

	message := fmt.Sprintf(format, args)

	if outputJSON {
		outbuf, _ := json.Marshal(JSON{Source: source, Message: message})
		fmt.Fprintln(om.Context.Out, string(outbuf))
		return
	} else {
		fmt.Fprintln(om.Context.Out, message)
	}
}
