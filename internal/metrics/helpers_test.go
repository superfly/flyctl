package metrics

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/superfly/flyctl/internal/config"
)

func queueForTest(t *testing.T) context.Context {
	t.Helper()

	prevEnabled, prevQueue, prevUnmatched := Enabled, metrics, unmatchedStatuses
	Enabled = true
	metrics = make([]metricsMessage, 0)
	unmatchedStatuses = map[string]struct{}{}
	t.Cleanup(func() {
		Enabled, metrics, unmatchedStatuses = prevEnabled, prevQueue, prevUnmatched
	})

	return config.NewContext(context.Background(), &config.Config{
		SendMetrics:    true,
		MetricsBaseURL: "http://127.0.0.1:0",
	})
}

func queuedSlugs() []string {
	slugs := make([]string, 0, len(metrics))
	for _, m := range metrics {
		slugs = append(slugs, m.Metric)
	}

	return slugs
}

func TestDeployFailedBeforeMachinesSendsStatusWithError(t *testing.T) {
	ctx := queueForTest(t)

	DeployFailedBeforeMachines(ctx, DeployStatusPayload{
		Error:   "failed to fetch an image or build from source: boom",
		AppID:   "123",
		OrgID:   "456",
		AppName: "my-app",
	})

	slugs := queuedSlugs()
	if len(slugs) != 2 || slugs[0] != "deploy/started" || slugs[1] != "deploy/status" {
		t.Fatalf("queued %v, want [deploy/started deploy/status]", slugs)
	}

	var got DeployStatusPayload
	if err := json.Unmarshal(metrics[1].Payload, &got); err != nil {
		t.Fatal(err)
	}
	if got.Error != "failed to fetch an image or build from source: boom" || got.OrgID != "456" || got.AppID != "123" {
		t.Fatalf("payload = %+v", got)
	}
}

func TestDeployFailedBeforeMachinesSendsOneStatusWhenAlreadyStarted(t *testing.T) {
	ctx := queueForTest(t)

	Started(ctx, "deploy")
	DeployFailedBeforeMachines(ctx, DeployStatusPayload{Error: "boom"})
	DeployStatus(ctx, DeployStatusPayload{Error: "again"})

	slugs := queuedSlugs()
	if len(slugs) != 2 || slugs[0] != "deploy/started" || slugs[1] != "deploy/status" {
		t.Fatalf("queued %v, want one started and one status", slugs)
	}
}
