package synthetics

import (
	"bytes"
	"context"
	"encoding/gob"
	"encoding/json"
	"os"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/prometheus/blackbox_exporter/config"
	"github.com/prometheus/blackbox_exporter/prober"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/superfly/flyctl/internal/logger"
	"nhooyr.io/websocket"
)

func RunAgent(ctx context.Context) error {
	logger := logger.FromContext(ctx)
	logger.Info("running synthetics agent")
	ws, err := NewMetricsWs()
	if err != nil {
		return err
	}

	if err = ws.Connect(ctx); err != nil {
		logger.Errorf("error connecting to flynthetics: %v", err)
		return err
	}

	go func() {
		defer ws.wsConn.CloseNow()
		c := make(chan os.Signal, 1)
		signalChannel(c)

		tick := time.NewTicker(5 * time.Second)
		defer tick.Stop()

		reconnectAt := time.Time{}

		for {
			select {
			case <-tick.C:
				if !reconnectAt.IsZero() && reconnectAt.Before(time.Now()) {
					ws.lock.Lock()
					ws.Connect(ctx)
					ws.lock.Unlock()

					reconnectAt = time.Time{}
				}

			case <-ctx.Done():
				return

			case <-c:
				if reconnectAt.IsZero() {
					reconnectAt = time.Now().Add(5 * time.Second)
				}

			case <-ws.reset:
				if reconnectAt.IsZero() {
					reconnectAt = time.Now().Add(5 * time.Second)
				}
			}
		}
	}()

	go listen(ctx, ws)

	return nil
}

func listen(ctx context.Context, ws *SyntheticsWs) error {
	logger := logger.FromContext(ctx)
	logger.Debug("start listening for probes")
	for ctx.Err() == nil {
		ws.lock.RLock()
		c := ws.wsConn
		ws.lock.RUnlock()

		_, message, err := c.Read(ctx)
		if err != nil {
			logger.Error("read error: ", err)
			ws.resetConn(c, err)
			continue
		}

		logger.Debug("received from server", string(message))

		err = processProbe(ctx, message, ws)
		if err != nil {
			logger.Error("failed processing probe", err)
		}

	}
	logger.Debug("stop listening for probes")

	return ctx.Err()
}

type ProbeMessage struct {
	Module string `json:"module"`
	Target string `json:"target"`
}

func processProbe(ctx context.Context, jsonMessage []byte, ws *SyntheticsWs) error {
	logger := logger.FromContext(ctx)
	logger.Debug("proccessing probes")

	probeMessage := ProbeMessage{}
	err := json.Unmarshal(jsonMessage, &probeMessage)
	if err != nil {
		logger.Error("JSON parse error:", err)
		return err
	}

	probeSuccessGauge := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "probe_success",
		Help: "Displays whether or not the probe was a success",
	})
	probeDurationGauge := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "probe_duration_seconds",
		Help: "Returns how long the probe took to complete in seconds",
	})

	module := config.Module{}
	module.HTTP.IPProtocol = "ipv4"

	var logBuf bytes.Buffer
	sl := log.NewLogfmtLogger(log.NewSyncWriter(&logBuf))

	start := time.Now()
	registry := prometheus.NewRegistry()
	registry.MustRegister(probeSuccessGauge)
	registry.MustRegister(probeDurationGauge)
	success := prober.ProbeHTTP(ctx, probeMessage.Target, module, registry, sl)
	duration := time.Since(start).Seconds()
	probeDurationGauge.Set(duration)

	if success {
		probeSuccessGauge.Set(1)
		level.Info(sl).Log("msg", "Probe succeeded", "duration_seconds", duration)
	} else {
		level.Error(sl).Log("msg", "Probe failed", "duration_seconds", duration)
	}

	mfs, err := registry.Gather()
	if err != nil {
		return err
	}

	var buf bytes.Buffer
	encoder := gob.NewEncoder(&buf)
	err = encoder.Encode(mfs)
	if err != nil {
		return err
	}

	data := buf.Bytes()

	ws.lock.RLock()
	c := ws.wsConn
	ws.lock.RUnlock()

	err = c.Write(ctx, websocket.MessageBinary, data)
	if err != nil {
		logger.Error("write error: ", err)
		ws.resetConn(c, err)
		return err
	}
	logger.Debugf("probe result sent to server. log: %s", &logBuf)

	return nil
}
