package recipe

import (
	"context"

	"github.com/superfly/flyctl/api"
)

type OperationType string

const (
	OperationTypeHTTP  = "http"
	OperationTypeFlaps = "flaps"
	OperationTypeSSH   = "ssh"
)

type GraphQLCommand struct {
	Endpoint string
	Args     []interface{}
}

type FlapsCommand struct {
	Method  string
	Action  string
	Options map[string]string
}

// func (f *Client) Restart(ctx context.Context, in api.RestartMachineInput) (err error) {
// 	restartEndpoint := fmt.Sprintf("/%s/restart?force_stop=%t", in.ID, in.ForceStop)

// 	if in.Timeout != 0 {
// 		restartEndpoint += fmt.Sprintf("&timeout=%d", in.Timeout)
// 	}

// 	if in.Signal != nil {
// 		restartEndpoint += fmt.Sprintf("&signal=%s", in.Signal)
// 	}

// 	if err := f.sendRequest(ctx, http.MethodPost, restartEndpoint, nil, nil, nil); err != nil {
// 		return fmt.Errorf("failed to restart VM %s: %w", in.ID, err)
// 	}
// 	return
// }

type MachineCommands interface {
	Launch(ctx context.Context, builder api.LaunchMachineInput) (*api.Machine, error)
}

type SSHCommand struct {
	Command string
}

type HTTPCommand struct {
	Method   string
	Endpoint string
	Port     int
	Data     map[string]string
}

type WaitForDefinition struct {
	HealthCheck HealthCheckSelector
}

type Operation struct {
	Name                string
	Type                OperationType
	FlapsCommand        FlapsCommand
	HTTPCommand         HTTPCommand
	SSHCommand          SSHCommand
	HealthCheckSelector HealthCheckSelector
	WaitForHealthChecks bool
}

type RecipeTemplate struct {
	Name         string
	App          *api.AppCompact
	RequireLease bool
	Operations   []Operation
}

type HealthCheckSelector struct {
	Name  string
	Value string
}
