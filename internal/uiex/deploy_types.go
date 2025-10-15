package uiex

import "encoding/json"

// TODO(AG): Error states

type DeploymentEventType string

const (
	DeploymentEventTypeStarted  DeploymentEventType = "started"
	DeploymentEventTypeProgress DeploymentEventType = "progress"
	DeploymentEventTypeSuccess  DeploymentEventType = "success"
	DeploymentEventTypeError    DeploymentEventType = "error"
)

type DeploymentEvent struct {
	Timestamp int64               `json:"timestamp"`
	Type      DeploymentEventType `json:"type"`
	Data      DeploymentEventData `json:"data"`
}

type DeploymentEventSkeleton struct {
	Timestamp int64               `json:"timestamp"`
	Type      DeploymentEventType `json:"type"`
	Data      json.RawMessage     `json:"data"`
}

type DeploymentEventData interface{ EventType() DeploymentEventType }

type DeploymentEventStarted struct{}

func (e DeploymentEventStarted) EventType() DeploymentEventType {
	return DeploymentEventTypeStarted
}

type DeploymentEventSuccess struct{}

func (e DeploymentEventSuccess) EventType() DeploymentEventType {
	return DeploymentEventTypeSuccess
}

type DeploymentEventError string

func (e DeploymentEventError) EventType() DeploymentEventType {
	return DeploymentEventTypeError
}

type DeploymentProgressType string

const (
	DeploymentProgressTypeInfo   DeploymentProgressType = "info"
	DeploymentProgressTypeUpdate DeploymentProgressType = "update"
	DeploymentProgressTypePlan   DeploymentProgressType = "plan"
)

type DeploymentProgressSkeleton struct {
	Type DeploymentProgressType `json:"type"`
	Data json.RawMessage        `json:"data"`
}

type DeploymentProgress struct {
	Type DeploymentProgressType `json:"type"`
	Data DeploymentProgressData `json:"data"`
}

func (d DeploymentProgress) EventType() DeploymentEventType {
	return DeploymentEventTypeProgress
}

type DeploymentProgressData interface {
	DataType() DeploymentProgressType
}

type DeploymentProgressInfo string

func (d DeploymentProgressInfo) DataType() DeploymentProgressType {
	return DeploymentProgressTypeInfo
}

type DeploymentProgressUpdate struct {
	Type          string `json:"type"`
	MachineID     string `json:"machine_id"`
	ProcessGroup  string `json:"process_group"`
	State         string `json:"state"`
	PassingChecks int    `json:"passing_checks"`
	TotalChecks   int    `json:"total_checks"`
}

func (d DeploymentProgressUpdate) DataType() DeploymentProgressType {
	return DeploymentProgressTypeUpdate
}

type DeploymentProgressPlan struct {
	Create int `json:"create"`
	Update int `json:"update"`
	Delete int `json:"delete"`
}

func (d DeploymentProgressPlan) DataType() DeploymentProgressType {
	return DeploymentProgressTypePlan
}

func UnmarshalDeploymentEvent(data []byte) (*DeploymentEvent, error) {
	var skel DeploymentEventSkeleton
	var evt DeploymentEvent

	if err := json.Unmarshal(data, &skel); err != nil {
		return nil, err
	}

	evt.Type = skel.Type
	evt.Timestamp = skel.Timestamp

	switch evt.Type {
	case DeploymentEventTypeStarted:
		var started DeploymentEventStarted
		evt.Data = &started
	case DeploymentEventTypeProgress:
		progress, err := UnmarshalDeploymentProgress(skel.Data)
		if err != nil {
			return nil, err
		}
		evt.Data = progress
	case DeploymentEventTypeSuccess:
		var success DeploymentEventSuccess
		evt.Data = &success
	case DeploymentEventTypeError:
		var deployErr DeploymentEventError
		if err := json.Unmarshal(skel.Data, &deployErr); err != nil {
			return nil, err
		}
		evt.Data = deployErr
	}

	return &evt, nil
}

func UnmarshalDeploymentProgress(data []byte) (*DeploymentProgress, error) {
	var skel DeploymentProgressSkeleton
	var progress DeploymentProgress

	if err := json.Unmarshal(data, &skel); err != nil {
		return nil, err
	}

	progress.Type = skel.Type

	switch progress.Type {
	case DeploymentProgressTypeInfo:
		var info DeploymentProgressInfo
		if err := json.Unmarshal(skel.Data, &info); err != nil {
			return nil, err
		}
		progress.Data = info
	case DeploymentProgressTypeUpdate:
		var update DeploymentProgressUpdate
		if err := json.Unmarshal(skel.Data, &update); err != nil {
			return nil, err
		}
		progress.Data = update
	case DeploymentProgressTypePlan:
		var plan DeploymentProgressPlan
		if err := json.Unmarshal(skel.Data, &plan); err != nil {
			return nil, err
		}
		progress.Data = plan
	}

	return &progress, nil
}
