package recipe

import (
	"github.com/superfly/flyctl/api"
)

type CommandType string

const (
	CommandTypeHTTP       = "http"
	CommandTypeFlaps      = "flaps"
	CommandTypeSSHConnect = "ssh_connect"
	CommandTypeSSHCommand = "ssh_command"
	CommandTypeGraphql    = "graphql"
	CommandTypeCustom     = "custom"
)

type HTTPCommandResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Data    string `json:"data"`
}

type FlapsCommand struct {
	Method  string
	Action  string
	Options map[string]string
}

type SSHRunCommand struct {
	App     *api.AppCompact
	Command string
}

type SSHConnectCommand struct {
	App     *api.AppCompact
	Command string
}

type HTTPCommand struct {
	Method   string
	Endpoint string
	Port     int
	Data     map[string]interface{}
	Result   interface{}
}

type GraphQLCommand struct {
	Query     string
	Variables map[string]interface{}
	Result    *api.Query
}

type WaitForDefinition struct {
	HealthCheck HealthCheckSelector
}

type PromptDefinition struct {
	Message string
}

type Selector struct {
	HealthCheck HealthCheckSelector
}

type CustomCommand func() error

type Operation struct {
	Name                string
	Prompt              PromptDefinition
	Type                CommandType
	FlapsCommand        FlapsCommand
	HTTPCommand         HTTPCommand
	SSHConnectCommand   SSHConnectCommand
	SSHRunCommand       SSHRunCommand
	CustomCommand       CustomCommand
	GraphQLCommand      GraphQLCommand
	Selector            Selector
	WaitForHealthChecks bool
}

type RecipeTemplate struct {
	Name         string
	App          *api.AppCompact
	RequireLease bool
	Operations   []*Operation
}

type HealthCheckSelector struct {
	Name  string
	Value string
}
