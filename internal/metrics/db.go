package metrics

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"

	"github.com/superfly/flyctl/agent"
	"github.com/superfly/flyctl/iostreams"
)

var metrics []metricsMessage = make([]metricsMessage, 0)

func queueMetric(metric metricsMessage) {
	metrics = append(metrics, metric)
}

// Spawns a forked `flyctl metrics send` process that sends metrics to the flyctl-metrics server
func FlushMetrics(ctx context.Context) error {
	json, err := json.Marshal(metrics)
	if err != nil {
		return err

	}

	flyctl, err := os.Executable()
	if err != nil {
		return err
	}

	cmd := exec.Command(flyctl, "metrics", "send")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}

	go func() {
		defer stdin.Close()
		io.WriteString(stdin, string(json))
	}()

	cmd.Env = os.Environ()
	cmd.Env = append(cmd.Env, "FLY_NO_UPDATE_CHECK=1")
	cmd.Env = append(cmd.Env, "FLY_NO_METRICS=1")

	agent.SetSysProcAttributes(cmd)

	if err := cmd.Run(); err != nil {
		return err
	}

	io := iostreams.FromContext(ctx)

	// On CI, always block on metrics send. This sucks, but the alternative is not getting metrics from CI at all. There are timeouts in place to prevent this from taking more than 15 seconds
	if io.IsInteractive() {
		if err := cmd.Process.Release(); err != nil {
			return err
		}
	} else {
		if err := cmd.Wait(); err != nil {
			return err
		}
	}

	return nil
}
