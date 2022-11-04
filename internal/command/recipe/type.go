package recipe

import "github.com/superfly/flyctl/api"

type OperationType string

const (
	OperationTypeFlaps   = "flaps"
	OperationTypeMachine = "machine"
)

type MachineCommand struct {
	Action string
}

type FlapsCommand struct {
}

type Operation struct {
	Name                string
	Type                OperationType
	Monitor             bool
	MachineCommand      MachineCommand
	FlapsCommand        FlapsCommand
	HealthCheckSelector HealthCheckSelector
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
