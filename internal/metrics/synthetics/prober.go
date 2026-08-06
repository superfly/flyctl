package synthetics

import (
	"context"
	"log/slog"
	"time"

	"github.com/prometheus/blackbox_exporter/config"
	"github.com/prometheus/blackbox_exporter/prober"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

func probeHTTP(ctx context.Context, probeMessage ProbeMessage, sl *slog.Logger) (mfs []*dto.MetricFamily, err error) {
	probeSuccessGauge := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "probe_success",
		Help: "Displays whether or not the probe was a success",
	})
	probeDurationGauge := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "probe_duration_seconds",
		Help: "Returns how long the probe took to complete in seconds",
	})

	module := config.Module{}
	module.HTTP = config.DefaultHTTPProbe

	module.HTTP.IPProtocol = probeMessage.IPProtocol

	start := time.Now()
	registry := prometheus.NewRegistry()
	registry.MustRegister(probeSuccessGauge)
	registry.MustRegister(probeDurationGauge)
	success := prober.ProbeHTTP(ctx, probeMessage.Target, module, registry, sl)
	duration := time.Since(start).Seconds()
	probeDurationGauge.Set(duration)

	if success {
		probeSuccessGauge.Set(1)
		sl.Info("Probe succeeded", "duration_seconds", duration)
	} else {
		sl.Error("Probe failed", "duration_seconds", duration)
	}

	mfs, err = registry.Gather()
	if err != nil {
		return nil, err
	}

	return mfs, nil
}
