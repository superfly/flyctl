package metrics

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
)

var (
	connExists sync.Mutex
	conn       *http.Client
)

func ensureConnection() {
	connExists.Lock()
	defer connExists.Unlock()
	if conn == nil {
		conn = &http.Client{}
	}
}

func Send[T any](metricSlug string, value T) error {
	ensureConnection()

	valJson, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return SendJson(metricSlug, string(valJson))
}

func SendNoData(metricSlug string) error {

	return SendJson(metricSlug, "")
}

func SendJson(metricSlug, jsonValue string) error {

	reader := strings.NewReader(jsonValue)

	hostname := "flyctl-metrics.fly.dev"
	if envHostname := os.Getenv("FLYCTL_METRICS_HOST"); envHostname != "" {
		hostname = envHostname
	}
	resp, err := conn.Post("https://"+hostname+"/v1/"+metricSlug, "application/json", reader)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("metrics server returned status code %d", resp.StatusCode)
	}
	return nil
}
