package recipe

import (
	"time"

	"github.com/superfly/flyctl/api"
)

type CommandType string

const (
	CommandTypeHTTP       = "http"
	CommandTypeFlaps      = "flaps"
	CommandTypeSSHConnect = "ssh_connect"
	CommandTypeSSHCommand = "ssh_command"
	CommandTypeGraphql    = "graphql"
	CommandTypeWaitFor    = "wait_for"
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

type WaitForCommand struct {
	HealthCheck HealthCheckSelector

	Retries  int
	Interval time.Duration
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
	WaitForCommand    WaitForCommand
	CustomCommand     CustomCommand

	Selector            Selector
	WaitForHealthChecks bool

	Targets []*api.Machine
}

type Selector struct {
	HealthCheck HealthCheckSelector
	Preprocess  bool
}

type Constraints struct {
	AppRoleID       string
	PlatformVersion string
	Images          []ImageConstraints
}

type ImageConstraints struct {
	Registry      string
	Repository    string
	MinFlyVersion string
}

type RecipeTemplate struct {
	Name         string
	App          *api.AppCompact
	RequireLease bool
	Operations   []*Operation
	Constraints  Constraints
}

type HealthCheckSelector struct {
	Name  string
	Value string
}
