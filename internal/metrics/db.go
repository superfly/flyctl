package metrics

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/superfly/flyctl/internal/buildinfo"
	"github.com/superfly/flyctl/internal/config"
)

// TODO(billy): Obviously, we should actually use an sqlite DB
var inMemoryMetricsDB []metricsMessage = make([]metricsMessage, 0)

func insertMetricToDB(metric metricsMessage) error {
	inMemoryMetricsDB = append(inMemoryMetricsDB, metric)
	return nil
}

// TODO: this should be done by the agent
// TODO(billy): Actually clear the DB
func FlushMetricsDB(ctx context.Context) error {
	json, err := json.Marshal(inMemoryMetricsDB)
	if err != nil {
		return err
	}

	authToken, err := getMetricsToken(ctx)
	if err != nil {
		return err
	}

	cfg := config.FromContext(ctx)
	request, err := http.NewRequest("POST", cfg.MetricsBaseURL+"/metrics_post", bytes.NewBuffer(json))
	if err != nil {
		return err
	}

	request.Header.Set("Authorization", authToken)
	request.Header.Set("User-Agent", fmt.Sprintf("flyctl/%s", buildinfo.Info().Version))

	client := &http.Client{}

	resp, err := client.Do(request)
	if err != nil {
		return err
	}

	return resp.Body.Close()
}
