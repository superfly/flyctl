package metrics

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
)

const (
	metricsToken = "abcd"
)

func sendImpl(metricSlug, jsonValue string) error {

	reader := strings.NewReader(jsonValue)

	hostname := "flyctl-metrics.fly.dev"
	if envHostname := os.Getenv("FLYCTL_METRICS_HOST"); envHostname != "" {
		hostname = envHostname
	}
	req, err := http.NewRequest("post", "https://"+hostname+"/v1/"+metricSlug, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+metricsToken)
	resp, err := http.DefaultClient.Do(req)
	defer func() {
		if resp != nil {
			_ = resp.Body.Close()
		}
	}()
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("metrics server returned status code %d", resp.StatusCode)
	}
	return nil
}

func Started(metricSlug string) {
	SendNoData(metricSlug + "/started")
}
func Status(metricSlug string, success bool) {
	Send(metricSlug+"/status", map[string]bool{"success": success})
}

func Send[T any](metricSlug string, value T) {

	valJson, err := json.Marshal(value)
	if err != nil {
		return
	}
	SendJson(metricSlug, string(valJson))
}

func SendNoData(metricSlug string) {

	SendJson(metricSlug, "")
}

func SendJson(metricSlug, jsonValue string) {
	// TODO(ali): Should this ping sentry when it fails?
	_ = sendImpl(metricSlug, jsonValue)
}
