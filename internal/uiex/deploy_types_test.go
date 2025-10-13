package uiex

import (
	"testing"
)

func TestUnmarshalDeploymentEvent(t *testing.T) {
	// Table of test cases derived from the provided sample plus a few variants
	tests := []struct {
		name     string
		jsonLine string
		assert   func(t *testing.T, evt *DeploymentEvent, err error)
	}{
		{
			name:     "started event",
			jsonLine: `{"timestamp":1759749334,"type":"started"}`,
			assert: func(t *testing.T, evt *DeploymentEvent, err error) {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if evt == nil {
					t.Fatalf("nil event")
				}
				if evt.Timestamp != 1759749334 {
					t.Fatalf("timestamp mismatch: got %d", evt.Timestamp)
				}
				if evt.Type != DeploymentEventTypeStarted {
					t.Fatalf("event type mismatch: got %s", evt.Type)
				}
				if _, ok := evt.Data.(*DeploymentEventStarted); !ok {
					t.Fatalf("data type mismatch: expected *DeploymentEventStarted, got %T", evt.Data)
				}
			},
		},
		{
			name:     "progress info",
			jsonLine: `{"data":{"data":"Planning deployment","type":"info"},"timestamp":1759749335,"type":"progress"}`,
			assert: func(t *testing.T, evt *DeploymentEvent, err error) {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if evt.Type != DeploymentEventTypeProgress {
					t.Fatalf("event type mismatch: got %s", evt.Type)
				}
				p, ok := evt.Data.(*DeploymentProgress)
				if !ok {
					t.Fatalf("data type mismatch: expected *DeploymentProgress, got %T", evt.Data)
				}
				if p.Type != DeploymentProgressTypeInfo {
					t.Fatalf("progress type mismatch: got %s", p.Type)
				}
				info, ok := p.Data.(DeploymentProgressInfo)
				if !ok {
					t.Fatalf("progress data mismatch: expected DeploymentProgressInfo, got %T", p.Data)
				}
				if string(info) != "Planning deployment" {
					t.Fatalf("info content mismatch: got %q", string(info))
				}
			},
		},
		{
			name:     "progress update started waiting",
			jsonLine: `{"data":{"data":{"type":"starting_update","machine_id":"d8d3e56a9149e8","process_group":"app"},"type":"update"},"timestamp":1759749335,"type":"progress"}`,
			assert: func(t *testing.T, evt *DeploymentEvent, err error) {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				p, ok := evt.Data.(*DeploymentProgress)
				if !ok {
					t.Fatalf("data type mismatch: expected *DeploymentProgress, got %T", evt.Data)
				}
				if p.Type != DeploymentProgressTypeUpdate {
					t.Fatalf("progress type mismatch: got %s", p.Type)
				}
				u, ok := p.Data.(DeploymentProgressUpdate)
				if !ok {
					t.Fatalf("progress data mismatch: expected DeploymentProgressUpdate, got %T", p.Data)
				}
				if u.Type != "starting_update" || u.MachineID != "d8d3e56a9149e8" || u.ProcessGroup != "app" {
					t.Fatalf("update content mismatch: %+v", u)
				}
			},
		},
		{
			name:     "progress update healthy with checks",
			jsonLine: `{"data":{"data":{"type":"healthy","machine_id":"d8d3e56a9149e8","passing_checks":1,"total_checks":1},"type":"update"},"timestamp":1759749372,"type":"progress"}`,
			assert: func(t *testing.T, evt *DeploymentEvent, err error) {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				p, ok := evt.Data.(*DeploymentProgress)
				if !ok {
					t.Fatalf("data type mismatch: expected *DeploymentProgress, got %T", evt.Data)
				}
				u, ok := p.Data.(DeploymentProgressUpdate)
				if !ok {
					t.Fatalf("progress data mismatch: expected DeploymentProgressUpdate, got %T", p.Data)
				}
				if u.Type != "healthy" || u.MachineID != "d8d3e56a9149e8" {
					t.Fatalf("update content mismatch: %+v", u)
				}
				if u.PassingChecks != 1 || u.TotalChecks != 1 {
					t.Fatalf("checks mismatch: %+v", u)
				}
			},
		},
		{
			name:     "success event",
			jsonLine: `{"timestamp":1759749372,"type":"success"}`,
			assert: func(t *testing.T, evt *DeploymentEvent, err error) {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if evt.Type != DeploymentEventTypeSuccess {
					t.Fatalf("event type mismatch: got %s", evt.Type)
				}
				if _, ok := evt.Data.(*DeploymentEventSuccess); !ok {
					t.Fatalf("data type mismatch: expected *DeploymentEventSuccess, got %T", evt.Data)
				}
			},
		},
		{
			name:     "progress update acquiring lease",
			jsonLine: `{"data":{"data":{"type":"acquiring_lease","machine_id":"d8d3e56a9149e8","process_group":"app"},"type":"update"},"timestamp":1759749335,"type":"progress"}`,
			assert: func(t *testing.T, evt *DeploymentEvent, err error) {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				p, ok := evt.Data.(*DeploymentProgress)
				if !ok {
					t.Fatalf("data type mismatch: expected *DeploymentProgress, got %T", evt.Data)
				}
				if p.Type != DeploymentProgressTypeUpdate {
					t.Fatalf("progress type mismatch: got %s", p.Type)
				}
				u, ok := p.Data.(DeploymentProgressUpdate)
				if !ok {
					t.Fatalf("progress data mismatch: expected DeploymentProgressUpdate, got %T", p.Data)
				}
				if u.Type != "acquiring_lease" || u.MachineID != "d8d3e56a9149e8" || u.ProcessGroup != "app" {
					t.Fatalf("update content mismatch: %+v", u)
				}
			},
		},
		{
			name:     "progress update lease acquired",
			jsonLine: `{"data":{"data":{"type":"lease_acquired","machine_id":"d8d3e56a9149e8","process_group":"app"},"type":"update"},"timestamp":1759749336,"type":"progress"}`,
			assert: func(t *testing.T, evt *DeploymentEvent, err error) {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				p, ok := evt.Data.(*DeploymentProgress)
				if !ok {
					t.Fatalf("data type mismatch: expected *DeploymentProgress, got %T", evt.Data)
				}
				u, ok := p.Data.(DeploymentProgressUpdate)
				if !ok {
					t.Fatalf("progress data mismatch: expected DeploymentProgressUpdate, got %T", p.Data)
				}
				if u.Type != "lease_acquired" || u.MachineID != "d8d3e56a9149e8" || u.ProcessGroup != "app" {
					t.Fatalf("update content mismatch: %+v", u)
				}
			},
		},
		{
			name:     "progress update updated",
			jsonLine: `{"data":{"data":{"type":"updated","machine_id":"d8d3e56a9149e8","process_group":"app"},"type":"update"},"timestamp":1759749339,"type":"progress"}`,
			assert: func(t *testing.T, evt *DeploymentEvent, err error) {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				p, ok := evt.Data.(*DeploymentProgress)
				if !ok {
					t.Fatalf("data type mismatch: expected *DeploymentProgress, got %T", evt.Data)
				}
				u, ok := p.Data.(DeploymentProgressUpdate)
				if !ok {
					t.Fatalf("progress data mismatch: expected DeploymentProgressUpdate, got %T", p.Data)
				}
				if u.Type != "updated" || u.MachineID != "d8d3e56a9149e8" || u.ProcessGroup != "app" {
					t.Fatalf("update content mismatch: %+v", u)
				}
			},
		},
		{
			name:     "progress update lease released",
			jsonLine: `{"data":{"data":{"type":"lease_released","machine_id":"d8d3e56a9149e8","process_group":"app"},"type":"update"},"timestamp":1759749340,"type":"progress"}`,
			assert: func(t *testing.T, evt *DeploymentEvent, err error) {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				p, ok := evt.Data.(*DeploymentProgress)
				if !ok {
					t.Fatalf("data type mismatch: expected *DeploymentProgress, got %T", evt.Data)
				}
				u, ok := p.Data.(DeploymentProgressUpdate)
				if !ok {
					t.Fatalf("progress data mismatch: expected DeploymentProgressUpdate, got %T", p.Data)
				}
				if u.Type != "lease_released" || u.MachineID != "d8d3e56a9149e8" || u.ProcessGroup != "app" {
					t.Fatalf("update content mismatch: %+v", u)
				}
			},
		},
		{
			name:     "progress update waiting_for_started without state",
			jsonLine: `{"data":{"data":{"type":"waiting_for_started","machine_id":"d8d3e56a9149e8"},"type":"update"},"timestamp":1759749340,"type":"progress"}`,
			assert: func(t *testing.T, evt *DeploymentEvent, err error) {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				p, ok := evt.Data.(*DeploymentProgress)
				if !ok {
					t.Fatalf("data type mismatch: expected *DeploymentProgress, got %T", evt.Data)
				}
				u, ok := p.Data.(DeploymentProgressUpdate)
				if !ok {
					t.Fatalf("progress data mismatch: expected DeploymentProgressUpdate, got %T", p.Data)
				}
				if u.Type != "waiting_for_started" || u.MachineID != "d8d3e56a9149e8" {
					t.Fatalf("update content mismatch: %+v", u)
				}
				if u.State != "" {
					t.Fatalf("expected empty state, got %q", u.State)
				}
			},
		},
		{
			name:     "progress update waiting_for_started with state",
			jsonLine: `{"data":{"data":{"type":"waiting_for_started","state":"replacing","machine_id":"d8d3e56a9149e8"},"type":"update"},"timestamp":1759749340,"type":"progress"}`,
			assert: func(t *testing.T, evt *DeploymentEvent, err error) {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				p, ok := evt.Data.(*DeploymentProgress)
				if !ok {
					t.Fatalf("data type mismatch: expected *DeploymentProgress, got %T", evt.Data)
				}
				u, ok := p.Data.(DeploymentProgressUpdate)
				if !ok {
					t.Fatalf("progress data mismatch: expected DeploymentProgressUpdate, got %T", p.Data)
				}
				if u.Type != "waiting_for_started" || u.MachineID != "d8d3e56a9149e8" || u.State != "replacing" {
					t.Fatalf("update content mismatch: %+v", u)
				}
			},
		},
		{
			name:     "progress update started event within update",
			jsonLine: `{"data":{"data":{"type":"started","machine_id":"d8d3e56a9149e8"},"type":"update"},"timestamp":1759749347,"type":"progress"}`,
			assert: func(t *testing.T, evt *DeploymentEvent, err error) {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				p, ok := evt.Data.(*DeploymentProgress)
				if !ok {
					t.Fatalf("data type mismatch: expected *DeploymentProgress, got %T", evt.Data)
				}
				u, ok := p.Data.(DeploymentProgressUpdate)
				if !ok {
					t.Fatalf("progress data mismatch: expected DeploymentProgressUpdate, got %T", p.Data)
				}
				if u.Type != "started" || u.MachineID != "d8d3e56a9149e8" {
					t.Fatalf("update content mismatch: %+v", u)
				}
			},
		},
		{
			name:     "progress update waiting_for_healthy without counts",
			jsonLine: `{"data":{"data":{"type":"waiting_for_healthy","machine_id":"d8d3e56a9149e8"},"type":"update"},"timestamp":1759749347,"type":"progress"}`,
			assert: func(t *testing.T, evt *DeploymentEvent, err error) {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				p, ok := evt.Data.(*DeploymentProgress)
				if !ok {
					t.Fatalf("data type mismatch: expected *DeploymentProgress, got %T", evt.Data)
				}
				u, ok := p.Data.(DeploymentProgressUpdate)
				if !ok {
					t.Fatalf("progress data mismatch: expected DeploymentProgressUpdate, got %T", p.Data)
				}
				if u.Type != "waiting_for_healthy" || u.MachineID != "d8d3e56a9149e8" {
					t.Fatalf("update content mismatch: %+v", u)
				}
				if u.PassingChecks != 0 || u.TotalChecks != 0 {
					t.Fatalf("expected zero counts, got %+v", u)
				}
			},
		},
		{
			name:     "progress update waiting_for_healthy with counts",
			jsonLine: `{"data":{"data":{"type":"waiting_for_healthy","machine_id":"d8d3e56a9149e8","passing_checks":0,"total_checks":1},"type":"update"},"timestamp":1759749347,"type":"progress"}`,
			assert: func(t *testing.T, evt *DeploymentEvent, err error) {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				p, ok := evt.Data.(*DeploymentProgress)
				if !ok {
					t.Fatalf("data type mismatch: expected *DeploymentProgress, got %T", evt.Data)
				}
				u, ok := p.Data.(DeploymentProgressUpdate)
				if !ok {
					t.Fatalf("progress data mismatch: expected DeploymentProgressUpdate, got %T", p.Data)
				}
				if u.Type != "waiting_for_healthy" || u.MachineID != "d8d3e56a9149e8" {
					t.Fatalf("update content mismatch: %+v", u)
				}
				if u.PassingChecks != 0 || u.TotalChecks != 1 {
					t.Fatalf("counts mismatch: %+v", u)
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			evt, err := UnmarshalDeploymentEvent([]byte(tc.jsonLine))
			tc.assert(t, evt, err)
		})
	}
}
