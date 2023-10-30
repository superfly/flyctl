package metrics

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
)

var metrics []metricsMessage = make([]metricsMessage, 0)

func insertMetric(metric metricsMessage) {
	metrics = append(metrics, metric)
}

// FIXME: make a subprocess fork run this
func FlushMetricsDB(ctx context.Context) error {
	json, err := json.Marshal(metrics)
	if err != nil {
		return err

	}

	flyctl, err := os.Executable()
	cmd := exec.Command(flyctl, "metrics", "send")

	buffer := bytes.Buffer{}
	buffer.Write(json)

	cmd.Stdin = &buffer
	cmd.Env = os.Environ()

	err = cmd.Start()
	if err != nil {
		return err
	}

	err = cmd.Process.Release()
	if err != nil {
		return err
	}

	return nil
}
