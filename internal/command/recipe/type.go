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
	CommandTypeWait       = "wait"
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

type WaitCommand struct {
	HealthCheck HealthCheckSelector
}

type PromptDefinition struct {
	Message string
}

type CustomCommand func() error

type Operation struct {
	Name   string
	Prompt PromptDefinition
	Type   CommandType

	GraphQLCommand    GraphQLCommand
	FlapsCommand      FlapsCommand
	HTTPCommand       HTTPCommand
	SSHConnectCommand SSHConnectCommand
	SSHRunCommand     SSHRunCommand
	WaitCommand       WaitCommand
	CustomCommand     CustomCommand

	Selector            Selector
	WaitForHealthChecks bool

	Targets []*api.Machine
}

type Selector struct {
	HealthCheck HealthCheckSelector
	Preprocess  bool
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
